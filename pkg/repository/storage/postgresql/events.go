package postgresql

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func CreateIdempotentRecord(db *gorm.DB, record any) (bool, error) {
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	return result.RowsAffected == 1, result.Error
}
