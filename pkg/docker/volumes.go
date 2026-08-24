package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type VolumeSummary struct {
	Name        string   `json:"name"`
	Driver      string   `json:"driver"`
	Mountpoint  string   `json:"mountpoint"`
	Scope       string   `json:"scope"`
	Created     string   `json:"created_at"`
	SizeMB      float64  `json:"size_mb"`
	SizeDisplay string   `json:"size_display"`
	StatusTag   string   `json:"status_tag"` // "in_use", "used_stopped", "unused"
	Containers  []string `json:"containers"`
	Engine      string   `json:"engine"` // "podman" or "docker"
}

type RawVolume struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
	Scope      string `json:"Scope"`
	CreatedAt  string `json:"CreatedAt"`
}

type RawVolumeList struct {
	Volumes []RawVolume `json:"Volumes"`
}

func (c *Client) ListVolumes() ([]VolumeSummary, error) {
	// Parallel container inspect to map volume usage
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	runningVolumes := make(map[string]bool)
	stoppedVolumes := make(map[string]bool)
	volumeContainers := make(map[string][]string)
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Limit concurrency to 10 workers

	for _, ctr := range containers {
		wg.Add(1)
		go func(cid, cname, cstate string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sc := c.GetClientForContainer(cid)
			inspectBody, code, errInspect := sc.Get(fmt.Sprintf("/containers/%s/json", cid))
			if errInspect == nil && code == 200 {
				var details struct {
					Mounts []struct {
						Name string `json:"Name"`
						Type string `json:"Type"`
					} `json:"Mounts"`
				}
				if errUnmarshal := json.Unmarshal(inspectBody, &details); errUnmarshal == nil {
					mu.Lock()
					for _, m := range details.Mounts {
						if m.Name != "" {
							if cstate == "running" {
								runningVolumes[m.Name] = true
							} else {
								stoppedVolumes[m.Name] = true
							}
							volumeContainers[m.Name] = append(volumeContainers[m.Name], cname)
						}
					}
					mu.Unlock()
				}
			}
		}(ctr.ID, ctr.Name, ctr.State)
	}
	wg.Wait()

	var result []VolumeSummary
	seenNames := make(map[string]bool)

	for _, sc := range c.allClients {
		body, code, err := sc.Get("/volumes")
		if err != nil || code != 200 {
			continue
		}

		var rawVolumes []RawVolume
		trimmedBody := strings.TrimSpace(string(body))

		// Podman returns array `[...]`, Docker returns object `{"Volumes": [...]}`
		if strings.HasPrefix(trimmedBody, "[") {
			if errArr := json.Unmarshal(body, &rawVolumes); errArr != nil {
				continue
			}
		} else {
			var rawList RawVolumeList
			if errObj := json.Unmarshal(body, &rawList); errObj != nil {
				continue
			}
			rawVolumes = rawList.Volumes
		}

		engineName := sc.Engine()
		var clientVols = make([]VolumeSummary, len(rawVolumes))
		var wgSize sync.WaitGroup

		for idx, v := range rawVolumes {
			wgSize.Add(1)
			go func(i int, vol RawVolume) {
				defer wgSize.Done()

				statusTag := "unused"
				mu.Lock()
				if runningVolumes[vol.Name] {
					statusTag = "in_use"
				} else if stoppedVolumes[vol.Name] {
					statusTag = "used_stopped"
				}
				ctrs := volumeContainers[vol.Name]
				mu.Unlock()

				mount := vol.Mountpoint
				if mount == "" || !dirExists(mount) {
					candidates := []string{
						"/var/lib/containers/storage/volumes/" + vol.Name + "/_data",
						"/var/lib/docker/volumes/" + vol.Name + "/_data",
					}
					for _, cand := range candidates {
						if dirExists(cand) {
							mount = cand
							break
						}
					}
				}

				sizeBytes := calculateDirSize(mount)
				sizeMB := float64(sizeBytes) / (1024 * 1024)
				sizeDisplay := formatSize(sizeBytes)

				clientVols[i] = VolumeSummary{
					Name:        vol.Name,
					Driver:      vol.Driver,
					Mountpoint:  mount,
					Scope:       vol.Scope,
					Created:     vol.CreatedAt,
					SizeMB:      sizeMB,
					SizeDisplay: sizeDisplay,
					StatusTag:   statusTag,
					Containers:  ctrs,
					Engine:      engineName,
				}
			}(idx, v)
		}
		wgSize.Wait()

		for _, cv := range clientVols {
			key := engineName + ":" + cv.Name
			if !seenNames[key] {
				seenNames[key] = true
				result = append(result, cv)
			}
		}
	}

	return result, nil
}

func dirExists(path string) bool {
	if fi, err := os.Stat(path); err == nil {
		return fi.IsDir()
	}
	return false
}

func calculateDirSize(path string) int64 {
	if path == "" {
		return 0
	}
	var totalSize int64
	var fileCount int

	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
			// Cap at 10,000 files to prevent long-running walks on huge DB directories
			if fileCount > 10000 {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return totalSize
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024.0
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.2f GB", gb)
}

func (c *Client) RemoveVolume(name string, force bool) error {
	path := fmt.Sprintf("/volumes/%s?force=%t", name, force)
	var lastErr error
	for _, sc := range c.allClients {
		body, code, err := sc.Delete(path)
		if err == nil && (code == 204 || code == 200) {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("remove volume failed status %d: %s", code, string(body))
		}
	}
	return lastErr
}

func (c *Client) PruneVolumes() ([]byte, error) {
	var results []string
	for _, sc := range c.allClients {
		body, code, err := sc.Post("/volumes/prune", nil)
		if err == nil && code == 200 {
			results = append(results, string(body))
		}
	}
	if len(results) > 0 {
		return []byte(results[0]), nil
	}
	return nil, fmt.Errorf("prune volumes failed on all engines")
}
