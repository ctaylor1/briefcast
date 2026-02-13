package db

import (
	"errors"
	"time"

	"github.com/akhilrex/briefcast/internal/logging"
	"gorm.io/gorm"
)

type localMigration struct {
	Name  string
	Query string
}

var migrations = []localMigration{
	{
		Name:  "2020_11_03_04_42_SetDefaultDownloadStatus",
		Query: "update podcast_items set download_status=2 where download_path!='' and download_status=0",
	},
}

func RunMigrations() {
	for _, mig := range migrations {
		ExecuteAndSaveMigration(mig.Name, mig.Query)
	}
}
func ExecuteAndSaveMigration(name string, query string) error {
	var migration Migration
	result := DB.Where("name=?", name).First(&migration)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logging.Sugar().Infow("running database migration", "name", name)
		result = DB.Debug().Exec(query)
		if result.Error == nil {
			DB.Save(&Migration{
				Date: time.Now(),
				Name: name,
			})
		}
		return result.Error
	}
	return nil
}
