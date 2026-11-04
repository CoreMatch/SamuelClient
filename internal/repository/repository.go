package repository

import (
	"database/sql"
	"fmt"

	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
)

// The user/task/file repositories were retired on 2026-11-05 the same way as
// the logs table (ADR-0004 follow-up): nothing in the launcher reads or
// writes those tables, so the code was deleted and the tables are no longer
// created. No DROP migration is issued on purpose — databases that already
// contain users/tasks/files tables keep them inert on disk, which avoids any
// destructive migration.

// SettingRepository handles setting database operations
type SettingRepository struct{}

// NewSettingRepository creates a new SettingRepository
func NewSettingRepository() *SettingRepository {
	return &SettingRepository{}
}

// Set creates or updates a setting
func (r *SettingRepository) Set(key, value, settingType, description string) error {
	query := `INSERT INTO settings (key, value, type, description) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, type = ?, description = ?, updated_at = CURRENT_TIMESTAMP`
	_, err := database.GetDB().Exec(query, key, value, settingType, description, value, settingType, description)
	if err != nil {
		return fmt.Errorf("failed to set setting %q: %w", key, err)
	}
	return nil
}

// Get retrieves a setting by key
func (r *SettingRepository) Get(key string) (*models.Setting, error) {
	setting := &models.Setting{}
	query := `SELECT id, key, value, type, description, created_at, updated_at FROM settings WHERE key = ?`
	err := database.GetDB().QueryRow(query, key).Scan(
		&setting.ID, &setting.Key, &setting.Value, &setting.Type, &setting.Description, &setting.CreatedAt, &setting.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("setting not found: %w", err)
	}
	return setting, nil
}

// GetAll retrieves all settings
func (r *SettingRepository) GetAll() ([]models.Setting, error) {
	query := `SELECT id, key, value, type, description, created_at, updated_at FROM settings ORDER BY key`
	rows, err := database.GetDB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer rows.Close()

	var settings []models.Setting
	for rows.Next() {
		var s models.Setting
		if err := rows.Scan(&s.ID, &s.Key, &s.Value, &s.Type, &s.Description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, nil
}

// Delete deletes a setting
func (r *SettingRepository) Delete(key string) error {
	query := `DELETE FROM settings WHERE key = ?`
	if _, err := database.GetDB().Exec(query, key); err != nil {
		return fmt.Errorf("failed to delete setting %q: %w", key, err)
	}
	return nil
}

// JavaRepository handles Java installation database operations
type JavaRepository struct{}

// NewJavaRepository creates a new JavaRepository
func NewJavaRepository() *JavaRepository {
	return &JavaRepository{}
}

// Create creates a new Java installation
func (r *JavaRepository) Create(java *models.JavaInstallation) error {
	query := `INSERT INTO java_installations (path, friendly_name, version, is_default) VALUES (?, ?, ?, ?)`
	result, err := database.GetDB().Exec(query, java.Path, java.FriendlyName, java.Version, java.IsDefault)
	if err != nil {
		return fmt.Errorf("failed to create java installation: %w", err)
	}
	java.ID, _ = result.LastInsertId()
	return nil
}

// GetByID retrieves a Java installation by ID
func (r *JavaRepository) GetByID(id int64) (*models.JavaInstallation, error) {
	java := &models.JavaInstallation{}
	query := `SELECT id, path, friendly_name, version, is_default, created_at, updated_at FROM java_installations WHERE id = ?`
	err := database.GetDB().QueryRow(query, id).Scan(
		&java.ID, &java.Path, &java.FriendlyName, &java.Version, &java.IsDefault, &java.CreatedAt, &java.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("java installation not found: %w", err)
	}
	return java, nil
}

// GetAll retrieves all Java installations
func (r *JavaRepository) GetAll() ([]models.JavaInstallation, error) {
	query := `SELECT id, path, friendly_name, version, is_default, created_at, updated_at FROM java_installations ORDER BY created_at DESC`
	rows, err := database.GetDB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get java installations: %w", err)
	}
	defer rows.Close()

	var javas []models.JavaInstallation
	for rows.Next() {
		var java models.JavaInstallation
		if err := rows.Scan(&java.ID, &java.Path, &java.FriendlyName, &java.Version, &java.IsDefault, &java.CreatedAt, &java.UpdatedAt); err != nil {
			return nil, err
		}
		javas = append(javas, java)
	}
	return javas, nil
}

// GetDefault retrieves the default Java installation
func (r *JavaRepository) GetDefault() (*models.JavaInstallation, error) {
	java := &models.JavaInstallation{}
	query := `SELECT id, path, friendly_name, version, is_default, created_at, updated_at FROM java_installations WHERE is_default = 1`
	err := database.GetDB().QueryRow(query).Scan(
		&java.ID, &java.Path, &java.FriendlyName, &java.Version, &java.IsDefault, &java.CreatedAt, &java.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("default java installation not found: %w", err)
	}
	return java, nil
}

// Update updates a Java installation
func (r *JavaRepository) Update(java *models.JavaInstallation) error {
	query := `UPDATE java_installations SET path = ?, friendly_name = ?, version = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := database.GetDB().Exec(query, java.Path, java.FriendlyName, java.Version, java.IsDefault, java.ID); err != nil {
		return fmt.Errorf("failed to update java installation %d: %w", java.ID, err)
	}
	return nil
}

// SetDefault sets a Java installation as the default. It fails if the id
// does not exist; without that check the transaction would silently clear
// every default flag and leave no default at all.
func (r *JavaRepository) SetDefault(id int64) error {
	tx, err := database.GetDB().Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	_, err = tx.Exec(`UPDATE java_installations SET is_default = 0`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("clear default flags: %w", err)
	}

	result, err := tx.Exec(`UPDATE java_installations SET is_default = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("set default flag: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		tx.Rollback()
		return fmt.Errorf("java installation not found: %w", sql.ErrNoRows)
	}

	return tx.Commit()
}

// Delete deletes a Java installation
func (r *JavaRepository) Delete(id int64) error {
	query := `DELETE FROM java_installations WHERE id = ?`
	result, err := database.GetDB().Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete java installation %d: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("java installation not found: %w", sql.ErrNoRows)
	}
	return nil
}
