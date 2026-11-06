package models

import "time"

// The User, Task, File, Mod, ModLoader and DownloadTask models were retired
// on 2026-11-05 together with their repositories (see internal/repository):
// nothing in the launcher referenced them. Tables that already exist in
// deployed databases stay inert on disk; no DROP migration is issued.

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

	// JoinServer/JoinPort are launch-only fields (never persisted to the
	// settings table): when JoinServer is set the game starts directly into
	// that server via --server/--port (match handoff flow, INTEROP-SPEC §6).
	JoinServer string `json:"join_server,omitempty"`
	JoinPort   int    `json:"join_port,omitempty"`
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
