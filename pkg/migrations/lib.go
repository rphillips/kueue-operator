package migrations

import (
	"context"

	"github.com/blang/semver/v4"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"k8s.io/klog/v2"
)

type Preamble struct {
	IntroducedVersion string
	Name              string
}

func (p Preamble) GetIntroducedVersionAsSemVer() (*semver.Version, error) {
	return semver.New(p.IntroducedVersion)
}

func (p Preamble) GetName() string {
	return p.Name
}

type Migration interface {
	GetName() string
	GetIntroducedVersionAsSemVer() (*semver.Version, error)

	Migrate(ctx context.Context, cc *controllercmd.ControllerContext) error
}

func RunAllMigrations(ctx context.Context, cc *controllercmd.ControllerContext) error {
	migrations := []Migration{
		NewNoOpMigration0_13_0_0(),
	}

	if len(migrations) == 0 {
		return nil
	}

	klog.Info("Running migrations")
	for _, migration := range migrations {
		klog.InfoS("running migration", "name", migration.GetName())
		if err := migration.Migrate(ctx, cc); err != nil {
			klog.ErrorS(err, "migration failed", "name", migration.GetName())
			return err
		}
		klog.InfoS("Migration completed successfully", "name", migration.GetName())
	}
	return nil
}
