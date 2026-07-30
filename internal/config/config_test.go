package config

import (
	"os"
	"path/filepath"
	"testing"
)

const validXML = `<config>
	<master-config>
		<listen-address>:29000</listen-address>
		<local-address>192.168.1.1</local-address>
		<name>Test Master</name>
		<message-of-the-day>Hello</message-of-the-day>
		<masters>
			<master name="M1">m1.example.com:28000</master>
		</masters>
		<rate-limit>5</rate-limit>
		<master-query-time-seconds>90</master-query-time-seconds>
		<game-query-time-seconds>45</game-query-time-seconds>
		<build-packets-every-seconds>2</build-packets-every-seconds>
		<min-packet-size>16</min-packet-size>
	</master-config>
	<web-config>
		<name>Test Web</name>
		<listen-address>127.0.0.1:9090</listen-address>
		<debug>true</debug>
		<build-pages-every-seconds>3</build-pages-every-seconds>
	</web-config>
</config>`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ValidFile(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config.xml", validXML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Master.ListenAddress != ":29000" {
		t.Errorf("ListenAddress = %q, want %q", cfg.Master.ListenAddress, ":29000")
	}
	if cfg.Master.LocalAddress != "192.168.1.1" {
		t.Errorf("LocalAddress = %q, want %q", cfg.Master.LocalAddress, "192.168.1.1")
	}
	if cfg.Master.Name != "Test Master" {
		t.Errorf("Name = %q, want %q", cfg.Master.Name, "Test Master")
	}
	if cfg.Master.MOTD != "Hello" {
		t.Errorf("MOTD = %q, want %q", cfg.Master.MOTD, "Hello")
	}
	if cfg.Master.RateLimit != 5 {
		t.Errorf("RateLimit = %d, want 5", cfg.Master.RateLimit)
	}
	if cfg.Master.MasterQueryTimeSeconds != 90 {
		t.Errorf("MasterQueryTimeSeconds = %d, want 90", cfg.Master.MasterQueryTimeSeconds)
	}
	if cfg.Master.GameQueryTimeSeconds != 45 {
		t.Errorf("GameQueryTimeSeconds = %d, want 45", cfg.Master.GameQueryTimeSeconds)
	}
	if cfg.Master.BuildPacketsEverySecs != 2 {
		t.Errorf("BuildPacketsEverySecs = %d, want 2", cfg.Master.BuildPacketsEverySecs)
	}
	if cfg.Master.MinPacketSize != 16 {
		t.Errorf("MinPacketSize = %d, want 16", cfg.Master.MinPacketSize)
	}
	if len(cfg.Master.Masters.Masters) != 1 {
		t.Fatalf("Masters count = %d, want 1", len(cfg.Master.Masters.Masters))
	}
	if cfg.Master.Masters.Masters[0].Name != "M1" {
		t.Errorf("Master[0].Name = %q, want %q", cfg.Master.Masters.Masters[0].Name, "M1")
	}
	if cfg.Master.Masters.Masters[0].Address != "m1.example.com:28000" {
		t.Errorf("Master[0].Address = %q, want %q", cfg.Master.Masters.Masters[0].Address, "m1.example.com:28000")
	}
	if cfg.Web.Name != "Test Web" {
		t.Errorf("Web.Name = %q, want %q", cfg.Web.Name, "Test Web")
	}
	if cfg.Web.ListenAddress != "127.0.0.1:9090" {
		t.Errorf("Web.ListenAddress = %q, want %q", cfg.Web.ListenAddress, "127.0.0.1:9090")
	}
	if !cfg.Web.Debug {
		t.Error("Web.Debug = false, want true")
	}
	if cfg.Web.BuildPagesEverySecs != 3 {
		t.Errorf("Web.BuildPagesEverySecs = %d, want 3", cfg.Web.BuildPagesEverySecs)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.xml")
	if err == nil {
		t.Fatal("Load should return error for nonexistent file")
	}
}

func TestLoad_InvalidXML(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config.xml", "not xml {{{")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should return error for invalid XML")
	}
}

func TestReload_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "config.xml", validXML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	updatedXML := `<config>
		<master-config>
			<listen-address>:30000</listen-address>
			<name>Updated Master</name>
			<rate-limit>10</rate-limit>
		</master-config>
		<web-config>
			<listen-address>127.0.0.1:7070</listen-address>
		</web-config>
	</config>`
	writeFile(t, dir, "config.xml", updatedXML)

	if err := cfg.Reload(path); err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}

	cfg.RLock()
	defer cfg.RUnlock()
	if cfg.Master.ListenAddress != ":30000" {
		t.Errorf("After reload ListenAddress = %q, want %q", cfg.Master.ListenAddress, ":30000")
	}
	if cfg.Master.Name != "Updated Master" {
		t.Errorf("After reload Name = %q, want %q", cfg.Master.Name, "Updated Master")
	}
	if cfg.Master.RateLimit != 10 {
		t.Errorf("After reload RateLimit = %d, want 10", cfg.Master.RateLimit)
	}
	if cfg.Web.ListenAddress != "127.0.0.1:7070" {
		t.Errorf("After reload Web.ListenAddress = %q, want %q", cfg.Web.ListenAddress, "127.0.0.1:7070")
	}
}

func TestReload_NonexistentFile(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Reload("/nonexistent/path/config.xml"); err == nil {
		t.Fatal("Reload should return error for nonexistent file")
	}
}

func TestReload_InvalidXML(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config.xml", "not xml {{{")
	cfg := &Config{}
	if err := cfg.Reload(path); err == nil {
		t.Fatal("Reload should return error for invalid XML")
	}
}

func TestWriteDefault_CreatesFileAndReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.xml")

	cfg := WriteDefault(path)

	if cfg.Master.ListenAddress != ":28000" {
		t.Errorf("ListenAddress = %q, want %q", cfg.Master.ListenAddress, ":28000")
	}
	if cfg.Master.Name != "Tribes Master" {
		t.Errorf("Name = %q, want %q", cfg.Master.Name, "Tribes Master")
	}
	if cfg.Master.RateLimit != 2 {
		t.Errorf("RateLimit = %d, want 2", cfg.Master.RateLimit)
	}
	if cfg.Master.MasterQueryTimeSeconds != 120 {
		t.Errorf("MasterQueryTimeSeconds = %d, want 120", cfg.Master.MasterQueryTimeSeconds)
	}
	if cfg.Master.GameQueryTimeSeconds != 30 {
		t.Errorf("GameQueryTimeSeconds = %d, want 30", cfg.Master.GameQueryTimeSeconds)
	}
	if cfg.Master.GameExpireQueries != 2 {
		t.Errorf("GameExpireQueries = %d, want 2", cfg.Master.GameExpireQueries)
	}
	if cfg.Master.BuildPacketsEverySecs != 1 {
		t.Errorf("BuildPacketsEverySecs = %d, want 1", cfg.Master.BuildPacketsEverySecs)
	}
	if cfg.Master.MinPacketSize != 0 {
		t.Errorf("MinPacketSize = %d, want 0", cfg.Master.MinPacketSize)
	}
	if len(cfg.Master.Masters.Masters) != 6 {
		t.Errorf("Masters count = %d, want 6", len(cfg.Master.Masters.Masters))
	}
	if cfg.Web.ListenAddress != "127.0.0.1:8080" {
		t.Errorf("Web.ListenAddress = %q, want %q", cfg.Web.ListenAddress, "127.0.0.1:8080")
	}
	if cfg.Web.Debug {
		t.Error("Web.Debug = true, want false")
	}

	// Verify file was written and is loadable
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Failed to load written default config: %v", err)
	}
	if loaded.Master.Name != "Tribes Master" {
		t.Errorf("Loaded Name = %q, want %q", loaded.Master.Name, "Tribes Master")
	}
}

func TestWriteDefault_InvalidPath(t *testing.T) {
	cfg := WriteDefault("/nonexistent/dir/config.xml")
	// Should still return defaults even if file write fails
	if cfg.Master.ListenAddress != ":28000" {
		t.Errorf("ListenAddress = %q, want %q", cfg.Master.ListenAddress, ":28000")
	}
}

func TestRLockRUnlock(t *testing.T) {
	cfg := &Config{}
	cfg.Master.Name = "test"

	cfg.RLock()
	name := cfg.Master.Name
	cfg.RUnlock()

	if name != "test" {
		t.Errorf("Name = %q, want %q", name, "test")
	}
}
