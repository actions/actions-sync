package src

import (
	"context"

	"github.com/spf13/cobra"
)

type SyncFlags struct {
	PullFlags
	PushFlags
}

func (f *SyncFlags) Init(cmd *cobra.Command) {
	f.PullFlags.Init(cmd)
	f.PushFlags.Init(cmd)
}

func (f *SyncFlags) Validate() Validations {
	return f.PullFlags.Validate().Join(f.PushFlags.Validate())
}

func Sync(ctx context.Context, cacheDir string, flags *SyncFlags) error {
	if err := Pull(ctx, cacheDir, &flags.PullFlags); err != nil {
		return err
	}
	if err := Push(ctx, cacheDir, &flags.PushFlags); err != nil {
		return err
	}
	return nil
}
