package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meschbach/marvin/internal/slacker"
	"github.com/meschbach/marvin/internal/slacker/cron"
	robfigcron "github.com/meschbach/marvin/internal/slacker/cron/robfig"
	"github.com/meschbach/marvin/internal/slacker/storage"
	"github.com/slack-go/slack"
)

func Run(ctx context.Context, opts *Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	passphrase, err := opts.ResolvePassphrase()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}

	logStartupDiagnostics(cfg, opts)

	components, err := initializeComponents(ctx, opts, cfg, passphrase)
	if err != nil {
		return err
	}

	if components.Config.MultiTenant != nil && len(components.Config.MultiTenant.CronJobs) > 0 {
		scheduler := robfigcron.NewScheduler()
		dispatcher := slacker.NewCronDispatcher(components.SlackBot.GetQueryProcessor(), components.SlackBot.GetSessionManager(), components.SlackBot.GetConnection())
		userStorage := storage.NewMemoryUser()

		mediator := cron.NewMediator(scheduler, dispatcher, userStorage)

		for _, job := range components.Config.MultiTenant.CronJobs {
			channel, _, _, err := components.SlackBot.GetConnection().GetClient().OpenConversation(&slack.OpenConversationParameters{Users: []string{job.SendTo}})
			if err != nil {
				return fmt.Errorf("cron %q: failed to resolve user %s: %w", job.Title, job.SendTo, err)
			}

			trigger := &cron.Trigger{
				Spec:    job.Schedule,
				Target:  []string{job.SendTo, channel.ID},
				Message: job.Message,
			}

			userKey := storage.UserKey{
				UserID:  job.SendTo,
				Channel: channel.ID,
			}

			if _, err := mediator.Register(ctx, userKey, trigger); err != nil {
				return fmt.Errorf("cron %q: failed to register: %w", job.Title, err)
			}
		}

		if err := mediator.Start(ctx); err != nil {
			return fmt.Errorf("cron: failed to start: %w", err)
		}
	}

	fmt.Println("Validating Slack setup...")
	if err := components.SlackBot.ValidateSlackSetup(); err != nil {
		return fmt.Errorf("slack setup validation failed: %w", err)
	}
	fmt.Println("✓ Slack setup validation passed")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	botCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := components.SlackBot.StartSocketMode(botCtx); err != nil {
			fmt.Printf("Bot error: %v\n", err)
			cancel()
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down Slacker bot...")

	cancel()

	time.Sleep(10 * time.Second)

	var shutdownErrors []error

	if err := components.TenantToolSet.Shutdown(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shutting down tool set: %w", err))
	}

	if components.Observability != nil {
		if err := components.Observability.ShutdownGracefully(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("shutting down observability: %w", err))
		}
	}

	fmt.Println("Slacker bot stopped")

	return errors.Join(shutdownErrors...)
}
