package go_plan_it

import (
	"gorm.io/driver/sqlite" // Sqlite driver based on CGO
	"gorm.io/gorm"
)

func NewDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("gorm.db"), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, err
	}

	if err = db.AutoMigrate(&Chat{}); err != nil {
		return nil, err
	}
	return db, nil
}
