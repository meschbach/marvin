package slacker

import (
	"context"
	"time"

	"github.com/meschbach/marvin/internal/junk"
	"github.com/meschbach/marvin/internal/query"
	"github.com/meschbach/marvin/internal/slacker/cron"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type CronDispatcher struct {
	queryProcessor *QueryProcessor
	sessionManager *SessionManager
	connection     *SlackConnection
}

func NewCronDispatcher(qp *QueryProcessor, sm *SessionManager, conn *SlackConnection) *CronDispatcher {
	return &CronDispatcher{
		queryProcessor: qp,
		sessionManager: sm,
		connection:     conn,
	}
}

func (d *CronDispatcher) OnTrigger(ctx context.Context, t cron.Trigger) error {
	userID := t.Target[0]
	channelID := t.Target[1]

	ctx, span := tracer.Start(ctx, "slacker.CronDispatcher.OnTrigger",
		trace.WithAttributes(
			attribute.String("cron.spec", t.Spec),
			attribute.String("cron.user", userID),
			attribute.String("cron.channel", channelID),
		),
	)
	defer span.End()

	slackCtx := &SlackContext{
		UserID:    userID,
		ChannelID: channelID,
		TeamID:    d.connection.GetTeamID(),
	}

	userContext := &query.UserContext{
		UserID:      userID,
		SlackTeamID: slackCtx.TeamID,
	}

	session := d.sessionManager.GetOrCreateSession(ctx, userID, channelID, userContext)
	updater := NewSlackUpdater(d.connection.GetClient(), channelID, NewSlackFormatter(), session.Preferences)

	isInitialized := d.queryProcessor.tenantToolSet.IsInitialized()
	span.SetAttributes(attribute.Bool("queryProcessor.initialized", isInitialized))
	if !isInitialized {
		initCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if err := d.queryProcessor.tenantToolSet.Initialize(initCtx); err != nil {
			span.SetAttributes(attribute.Bool("tools.init.success", false))
			return err
		}
		span.SetAttributes(attribute.Bool("tools.init.success", true))
	}

	err := d.queryProcessor.HandleQueryWithUpdater(ctx, slackCtx, session, t.Message, updater)
	if err != nil {
		return junk.RecordSpanError(span, err)
	}
	span.SetAttributes(attribute.Bool("query.success", true))
	return nil
}
