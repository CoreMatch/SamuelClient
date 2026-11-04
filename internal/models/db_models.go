package models

import "time"

// User represents a user in the database
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Setting represents a setting in the database
type Setting struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Type        string    `json:"type"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Task represents a task in the database
type Task struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Progress    int        `json:"progress"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// File represents a file record in the database
type File struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type,omitempty"`
	Checksum  string    `json:"checksum,omitempty"`
	Metadata  string    `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Task status constants
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// User role constants
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// ==================== Minecraft Launcher Models ====================

// MinecraftAccount represents a Minecraft account
type MinecraftAccount struct {
	ID          int64     `json:"id"`
	UUID        string    `json:"uuid"`
	Username    string    `json:"username"`
	AccessToken string    `json:"access_token,omitempty"`
	ClientToken string    `json:"client_token,omitempty"`
	AuthType    string    `json:"auth_type"`             // "offline", "microsoft", "authlib-injector"
	AuthServer  string    `json:"auth_server,omitempty"` // For authlib-injector
	PlayerUUID  string    `json:"player_uuid,omitempty"`
	PlayerName  string    `json:"player_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MinecraftVersion represents a Minecraft version
type MinecraftVersion struct {
	ID            int64     `json:"id"`
	VersionID     string    `json:"version_id"`   // e.g., "1.20.4"
	VersionType   string    `json:"version_type"` // "release", "snapshot", "old_beta", "old_alpha"
	GameDir       string    `json:"game_dir,omitempty"`
	JarPath       string    `json:"jar_path,omitempty"`
	NativesDir    string    `json:"natives_dir,omitempty"`
	LibrariesDir  string    `json:"libraries_dir,omitempty"`
	MainClass     string    `json:"main_class,omitempty"`
	AssetIndex    string    `json:"asset_index,omitempty"`
	IsInstalled   bool      `json:"is_installed"`
	IsSelected    bool      `json:"is_selected"`
	LoaderType    string    `json:"loader_type,omitempty"` // "fabric", "forge", "none"
	LoaderVersion string    `json:"loader_version,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ModLoader represents a mod loader (Fabric or Forge)
type ModLoader struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"` // "fabric" or "forge"
	Version       string    `json:"version"`
	MCVersion     string    `json:"mc_version"` // Compatible Minecraft version
	JarPath       string    `json:"jar_path,omitempty"`
	InstallerPath string    `json:"installer_path,omitempty"`
	DownloadURL   string    `json:"download_url,omitempty"`
	MD5           string    `json:"md5,omitempty"`
	IsInstalled   bool      `json:"is_installed"`
	CreatedAt     time.Time `json:"created_at"`
}

// Mod represents a mod
type Mod struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	ModID       string    `json:"mod_id,omitempty"` // Fabric/Forge mod ID
	Version     string    `json:"version"`
	MCVersion   string    `json:"mc_version"`
	LoaderType  string    `json:"loader_type"` // "fabric", "forge", "universal"
	FilePath    string    `json:"file_path,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"`
	MD5         string    `json:"md5,omitempty"`
	FileSize    int64     `json:"file_size"`
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DownloadTask represents a download task
type DownloadTask struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DownloadURL string    `json:"download_url"`
	DestPath    string    `json:"dest_path"`
	FileSize    int64     `json:"file_size"`
	Downloaded  int64     `json:"downloaded"`
	Status      string    `json:"status"` // "pending", "downloading", "completed", "failed", "cancelled"
	Progress    int       `json:"progress"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LauncherConfig represents launcher configuration
type LauncherConfig struct {
	JavaPath       string `json:"java_path"`
	MaxMemory      int    `json:"max_memory"`      // MB
	MinMemory      int    `json:"min_memory"`      // MB
	JavaArgs       string `json:"java_args"`       // Additional JVM arguments
	GameWidth      int    `json:"game_width"`      // Window width
	GameHeight     int    `json:"game_height"`     // Window height
	GameDir        string `json:"game_dir"`        // .minecraft directory
	JavaDir        string `json:"java_dir"`        // Java installation directory
	BMCLAPIMirrors string `json:"bmclapi_mirrors"` // BMCLAPI mirror URLs
	AuthlibJar     string `json:"authlib_jar"`     // Absolute path to authlib-injector.jar (optional)
	// AuthlibMetaJSON is the raw metadata JSON returned by the authlib
	// server's GET / endpoint. When launching an authlib-injector account
	// it is Base64-encoded into -Dauthlibinjector.yggdrasil.prefetched per
	// the authlib-injector wiki (配置预获取). Leave empty to skip prefetching.
	AuthlibMetaJSON string `json:"authlib_meta_json,omitempty"`
}

// AuthServer represents an authlib-injector server
type AuthServer struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	IconURL   string    `json:"icon_url,omitempty"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

// Download task status constants
const (
	DownloadStatusPending     = "pending"
	DownloadStatusDownloading = "downloading"
	DownloadStatusCompleted   = "completed"
	DownloadStatusFailed      = "failed"
	DownloadStatusCancelled   = "cancelled"
)

// JavaInstallation represents a Java installation
type JavaInstallation struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`          // Path to java executable
	FriendlyName string    `json:"friendly_name"` // User-friendly name
	Version      string    `json:"version"`       // Detected Java version
	IsDefault    bool      `json:"is_default"`    // Is this the default Java
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
