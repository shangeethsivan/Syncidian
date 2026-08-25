package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Persistence describes whether SQLite, users, GitHub App credentials, and
// vault files in DataDir will survive the next container replace.
type Persistence struct {
	OK      bool   `json:"ok"`
	DataDir string `json:"data_dir"`
	Kind    string `json:"kind"` // volume, host, ephemeral
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

func PersistenceStatus(dataDir string) Persistence {
	p := Persistence{OK: true, DataDir: dataDir, Kind: "host"}
	volume := strings.TrimSpace(os.Getenv("RAILWAY_VOLUME_MOUNT_PATH"))
	if volume != "" {
		p.Kind = "volume"
		p.Message = "Users, GitHub App credentials, and vault files are stored on the attached volume."
		return p
	}
	if platform, hint := hostedWithoutVolume(); platform != "" {
		p.OK = false
		p.Kind = "ephemeral"
		p.Message = "This " + platform + " filesystem is wiped on every deploy. Users, GitHub configuration, and vault files will disappear until you attach a persistent volume."
		p.Hint = hint
		return p
	}
	if inContainer() && overlayOrTmpfs(dataDir) {
		p.OK = false
		p.Kind = "ephemeral"
		p.Message = "The data directory is on the container filesystem, which is reset when the container is replaced. Users, GitHub configuration, and vault files will not survive a new deploy."
		p.Hint = "Mount a named volume at /data (docker run -v syncidian-data:/data, docker compose, or Railway Settings → Volumes with mount path /data)."
		return p
	}
	return p
}

func hostedWithoutVolume() (platform, hint string) {
	switch {
	case envSet("RAILWAY_ENVIRONMENT", "RAILWAY_PROJECT_ID", "RAILWAY_SERVICE_ID"):
		return "Railway", "Railway: service → Settings → Volumes → Add volume, mount path /data. Then redeploy. railway.json requires that mount so new deploys fail closed instead of wiping the vault."
	case envSet("RENDER", "RENDER_SERVICE_ID"):
		return "Render", "Render: add a persistent disk mounted at /data, or set SYNCIDIAN_DATA to the disk mount path."
	case envSet("FLY_APP_NAME", "FLY_ALLOC_ID"):
		return "Fly.io", "Fly.io: create a volume and mount it at /data in fly.toml ([mounts] source = \"syncidian_data\" destination = \"/data\")."
	case envSet("DYNO"):
		return "Heroku", "Heroku dynos have an ephemeral filesystem. Run Syncidian on a host with a disk, or another PaaS that supports volumes."
	case envSet("COOLIFY_URL", "COOLIFY_FQDN"):
		return "Coolify", "Coolify: attach a persistent storage volume to /data for this service."
	case envSet("K_SERVICE"):
		return "Cloud Run", "Cloud Run storage is ephemeral. Mount a volume at /data or run Syncidian where the SQLite file can live on a disk."
	default:
		return "", ""
	}
}

func envSet(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return envSet("KUBERNETES_SERVICE_HOST")
}

func overlayOrTmpfs(dir string) bool {
	fsType := mountFSType(dir)
	switch fsType {
	case "overlay", "overlay2", "tmpfs", "ramfs", "aufs":
		return true
	default:
		return false
	}
}

func mountFSType(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return ""
	}
	defer f.Close()

	bestLen := -1
	fsType := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		mountPoint := unescapeMount(fields[1])
		if mountPoint == abs || strings.HasPrefix(abs, strings.TrimRight(mountPoint, "/")+"/") || mountPoint == "/" {
			if len(mountPoint) >= bestLen {
				bestLen = len(mountPoint)
				fsType = fields[2]
			}
		}
	}
	return fsType
}

func unescapeMount(s string) string {
	s = strings.ReplaceAll(s, `\040`, " ")
	s = strings.ReplaceAll(s, `\011`, "\t")
	s = strings.ReplaceAll(s, `\012`, "\n")
	s = strings.ReplaceAll(s, `\134`, `\`)
	return s
}
