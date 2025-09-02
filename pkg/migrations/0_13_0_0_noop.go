package migrations

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	clientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
)

type NoOpMigration0_13_0_0 struct {
	Preamble
}

func NewNoOpMigration0_13_0_0() Migration {
	return &NoOpMigration0_13_0_0{
		Preamble: Preamble{
			IntroducedVersion: "0.13",
			Name:              "NoOp",
		},
	}
}

func (c *NoOpMigration0_13_0_0) Migrate(ctx context.Context, cc *controllercmd.ControllerContext) error {
	_, err := clientset.NewForConfig(cc.KubeConfig)
	if err != nil {
		return fmt.Errorf("creating clientset failed: %w", err)
	}
	return nil
}
