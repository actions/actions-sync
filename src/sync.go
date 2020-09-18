package src

import (
	"context"

	"github.com/spf13/cobra"
)

type SyncFlags struct {
	CommonFlags
	PullOnlyFlags
	PushOnlyFlags
}

func (f *SyncFlags) Init(cmd *cobra.Command) {
	f.CommonFlags.Init(cmd)
	f.PullOnlyFlags.Init(cmd)
	f.PushOnlyFlags.Init(cmd)
}

func (f *SyncFlags) Validate() Validations {
	return f.CommonFlags.Validate(true).Join(f.PullOnlyFlags.Validate().Join(f.PushOnlyFlags.Validate()))
}

func Sync(ctx context.Context, flags *SyncFlags) error {

	pullFlags := &PullFlags{flags.CommonFlags, flags.PullOnlyFlags}
	pushFlags := &PushFlags{flags.CommonFlags, flags.PushOnlyFlags}

	if err := Pull(ctx, pullFlags); err != nil {
		return err
	}
	if err := Push(ctx, pushFlags); err != nil {
		return err
	}
	return nil
}
