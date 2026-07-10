package plugins

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestAutoInstallPluginsLocksSelectedPluginRows(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "autoinstall-lock-" + uuid.NewString()[:8],
		Name:     "Auto Install Lock",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"auto_install":true}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	selected := make(chan struct{})
	release := make(chan struct{})
	released := false
	releaseTransaction := func() {
		if !released {
			close(release)
			released = true
		}
	}
	t.Cleanup(releaseTransaction)

	transactionDone := make(chan error, 1)
	go func() {
		transactionDone <- db.Transaction(func(tx *gorm.DB) error {
			plugins, err := autoInstallPlugins(context.Background(), tx)
			if err != nil {
				return err
			}
			for _, selectedPlugin := range plugins {
				if selectedPlugin.ID == plugin.ID {
					close(selected)
					<-release
					return nil
				}
			}
			return fmt.Errorf("selected plugins did not include %s", plugin.ID)
		})
	}()

	select {
	case <-selected:
	case <-time.After(time.Second):
		t.Fatal("timed out selecting auto-install plugin")
	}

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}).Error
	}()

	select {
	case err := <-deleteDone:
		t.Fatalf("plugin delete completed while its row was locked: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseTransaction()
	if err := <-transactionDone; err != nil {
		t.Fatalf("select auto-install plugins: %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("delete plugin after lock release: %v", err)
	}
}
