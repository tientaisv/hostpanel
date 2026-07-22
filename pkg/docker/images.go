package docker

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ImageSummary struct {
	ID          string   `json:"id"`
	ShortID     string   `json:"short_id"`
	RepoTags    []string `json:"repo_tags"`
	TagDisplay  string   `json:"tag_display"`
	SizeMB      float64  `json:"size_mb"`
	Created     int64    `json:"created"`
	StatusTag   string   `json:"status_tag"` // "in_use", "used_stopped", "unused"
	Containers  []string `json:"containers"`
}

type RawImage struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	Size        int64    `json:"Size"`
	Created     int64    `json:"Created"`
}

func (c *Client) ListImages() ([]ImageSummary, error) {
	// First get list of all containers to map image usage
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	runningImageIDs := make(map[string]bool)
	runningImageNames := make(map[string]bool)
	stoppedImageIDs := make(map[string]bool)
	stoppedImageNames := make(map[string]bool)
	imageContainerMap := make(map[string][]string)

	for _, ctr := range containers {
		imgID := ctr.ImageID
		imgName := ctr.Image
		if ctr.State == "running" {
			runningImageIDs[imgID] = true
			runningImageNames[imgName] = true
		} else {
			stoppedImageIDs[imgID] = true
			stoppedImageNames[imgName] = true
		}
		imageContainerMap[imgID] = append(imageContainerMap[imgID], ctr.Name)
		imageContainerMap[imgName] = append(imageContainerMap[imgName], ctr.Name)
	}

	body, code, err := c.Get("/images/json")
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("list images failed status %d", code)
	}

	var rawList []RawImage
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, err
	}

	result := make([]ImageSummary, 0)
	for _, r := range rawList {
		shortID := r.ID
		if strings.HasPrefix(shortID, "sha256:") {
			shortID = shortID[7:]
		}
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		tagDisplay := "<none>:<none>"
		if len(r.RepoTags) > 0 && r.RepoTags[0] != "<none>:<none>" {
			tagDisplay = strings.Join(r.RepoTags, ", ")
		}

		// Determine usage tag
		statusTag := "unused"
		isInUse := runningImageIDs[r.ID]
		isUsedStopped := stoppedImageIDs[r.ID]

		for _, tag := range r.RepoTags {
			if runningImageNames[tag] {
				isInUse = true
			}
			if stoppedImageNames[tag] {
				isUsedStopped = true
			}
		}

		if isInUse {
			statusTag = "in_use"
		} else if isUsedStopped {
			statusTag = "used_stopped"
		}

		associatedCtrs := imageContainerMap[r.ID]

		result = append(result, ImageSummary{
			ID:         r.ID,
			ShortID:    shortID,
			RepoTags:   r.RepoTags,
			TagDisplay: tagDisplay,
			SizeMB:     float64(r.Size) / (1024 * 1024),
			Created:    r.Created,
			StatusTag:  statusTag,
			Containers: associatedCtrs,
		})
	}

	return result, nil
}

func (c *Client) RemoveImage(id string, force bool) error {
	path := fmt.Sprintf("/images/%s?force=%t", id, force)
	body, code, err := c.Delete(path)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("remove image failed status %d: %s", code, string(body))
	}
	return nil
}

func (c *Client) PruneImages() ([]byte, error) {
	body, code, err := c.Post("/images/prune", nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("prune images failed status %d: %s", code, string(body))
	}
	return body, nil
}
