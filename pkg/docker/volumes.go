package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Map volume usage via containers inspect
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	runningVolumes := make(map[string]bool)
	stoppedVolumes := make(map[string]bool)
	volumeContainers := make(map[string][]string)

	for _, ctr := range containers {
		inspectBody, code, errInspect := c.Get(fmt.Sprintf("/containers/%s/json", ctr.ID))
		if errInspect == nil && code == 200 {
			var details struct {
				Mounts []struct {
					Name string `json:"Name"`
					Type string `json:"Type"`
				} `json:"Mounts"`
			}
			if errUnmarshal := json.Unmarshal(inspectBody, &details); errUnmarshal == nil {
				for _, m := range details.Mounts {
					if m.Name != "" {
						if ctr.State == "running" {
							runningVolumes[m.Name] = true
						} else {
							stoppedVolumes[m.Name] = true
						}
						volumeContainers[m.Name] = append(volumeContainers[m.Name], ctr.Name)
					}
				}
			}
		}
	}

	body, code, err := c.Get("/volumes")
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list volumes status %d", code)
	}

	var rawList RawVolumeList
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, err
	}

	result := make([]VolumeSummary, 0)
	for _, v := range rawList.Volumes {
		statusTag := "unused"
		if runningVolumes[v.Name] {
			statusTag = "in_use"
		} else if stoppedVolumes[v.Name] {
			statusTag = "used_stopped"
		}

		sizeBytes := calculateDirSize(v.Mountpoint)
		sizeMB := float64(sizeBytes) / (1024 * 1024)
		sizeDisplay := formatSize(sizeBytes)

		result = append(result, VolumeSummary{
			Name:        v.Name,
			Driver:      v.Driver,
			Mountpoint:  v.Mountpoint,
			Scope:       v.Scope,
			Created:     v.CreatedAt,
			SizeMB:      sizeMB,
			SizeDisplay: sizeDisplay,
			StatusTag:   statusTag,
			Containers:  volumeContainers[v.Name],
		})
	}

	return result, nil
}

func calculateDirSize(path string) int64 {
	var totalSize int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
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
	body, code, err := c.Delete(path)
	if err != nil {
		return err
	}
	if code != 204 && code != 200 {
		return fmt.Errorf("remove volume failed status %d: %s", code, string(body))
	}
	return nil
}

func (c *Client) PruneVolumes() ([]byte, error) {
	body, code, err := c.Post("/volumes/prune", nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("prune volumes status %d: %s", code, string(body))
	}
	return body, nil
}
