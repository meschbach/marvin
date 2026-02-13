package query

import (
	"context"

	"github.com/meschbach/marvin/internal/config"
	"github.com/meschbach/marvin/internal/conversation"
)

func loadToolsFromConfig(ctx context.Context, cfg *config.File) (*conversation.ToolSet, error) {
	ts := conversation.NewToolSet()
	if cfg != nil {
		for _, lp := range cfg.LocalPrograms {
			t := FromLocalProgram(lp)
			ts.Container.Register(t)
			if err := ts.RegisterTool(ctx, t); err != nil {
				return nil, &localProgramDiscoveryError{
					name:       t.Name,
					underlying: err,
				} // fail hard per requirements
			}
		}
		if err := loadToolsFromDocker(ctx, ts, cfg); err != nil {
			return nil, err
		}
		if err := loadToolsFromHTTP(ctx, ts, cfg); err != nil {
			return nil, err
		}
	}
	return ts, nil
}
