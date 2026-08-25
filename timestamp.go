package gormkit

import (
	"time"

	"gorm.io/gorm"
)

// TimestampPlugin maintains configurable Unix-second timestamp columns.
// Empty fields use ctime and mtime. A nil Now function uses time.Now.
type TimestampPlugin struct {
	CreatedColumn string
	UpdatedColumn string
	Now           func() time.Time
}

// NewTimestampPlugin returns a timestamp plugin with conventional defaults.
func NewTimestampPlugin() *TimestampPlugin {
	return &TimestampPlugin{
		CreatedColumn: "ctime",
		UpdatedColumn: "mtime",
		Now:           time.Now,
	}
}

func (plugin *TimestampPlugin) Name() string { return "gormkit:timestamps" }

func (plugin *TimestampPlugin) Initialize(db *gorm.DB) error {
	created, updated, now := plugin.defaults()
	if err := db.Callback().Create().Before("gorm:before_create").Register("gormkit:timestamps:create", func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Schema == nil {
			return
		}
		unix := now().Unix()
		if _, ok := db.Statement.Schema.FieldsByDBName[created]; ok {
			db.Statement.SetColumn(created, unix)
		}
		if _, ok := db.Statement.Schema.FieldsByDBName[updated]; ok {
			db.Statement.SetColumn(updated, unix)
		}
	}); err != nil {
		return err
	}
	return db.Callback().Update().Before("gorm:before_update").Register("gormkit:timestamps:update", func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Schema == nil {
			return
		}
		if _, ok := db.Statement.Schema.FieldsByDBName[updated]; ok {
			db.Statement.SetColumn(updated, now().Unix())
		}
	})
}

func (plugin *TimestampPlugin) defaults() (string, string, func() time.Time) {
	created := plugin.CreatedColumn
	if created == "" {
		created = "ctime"
	}
	updated := plugin.UpdatedColumn
	if updated == "" {
		updated = "mtime"
	}
	now := plugin.Now
	if now == nil {
		now = time.Now
	}
	return created, updated, now
}

// NewTimeAutoUpdatePlugin is retained for source compatibility.
// Deprecated: use NewTimestampPlugin.
func NewTimeAutoUpdatePlugin() *TimestampPlugin {
	return NewTimestampPlugin()
}
