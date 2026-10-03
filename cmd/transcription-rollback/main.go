// Operator-only, dry-run by default. No application initialization or scheduler.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	puresqlite "github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	apply := flag.Bool("apply", false, "Reset only inspected unfinished remote envelopes")
	stopped := flag.Bool("services-stopped", false, "Operator confirms both split services are stopped")
	backup := flag.String("backup", "", "New private backup filename; required for apply")
	flag.Parse()
	if *apply && (!*stopped || *backup == "") {
		fail()
	}
	config, err := db.ResolveDatabaseConfig()
	if err != nil {
		fail()
	}
	var dialect gorm.Dialector
	if config.Driver == db.DriverPostgres {
		dialect = postgres.Open(config.DataSource)
	} else {
		// Require an existing persistent database; a dry run must not create a
		// new empty SQLite file or silently select an in-memory database.
		path := strings.TrimPrefix(strings.SplitN(config.DataSource, "?", 2)[0], "file:")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			fail()
		}
		dialect = puresqlite.Open(config.DataSource)
	}
	handle, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fail()
	}
	connection, err := handle.DB()
	if err != nil {
		fail()
	}
	defer connection.Close()
	counts, err := service.PrepareMonolithRollback(handle, *backup, *apply)
	if err != nil {
		fail()
	}
	_ = json.NewEncoder(os.Stdout).Encode(counts)
}
func fail() {
	fmt.Fprintln(os.Stderr, "Rollback stopped: verify database access, recognized envelopes, stopped services and a new private backup path.")
	os.Exit(1)
}
