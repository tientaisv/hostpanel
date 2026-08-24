package docker

import (
	"encoding/json"
	"fmt"
)

type NetworkContainerInfo struct {
	Name        string `json:"Name"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

type RawNetwork struct {
	ID         string                          `json:"Id"`
	Name       string                          `json:"Name"`
	Driver     string                          `json:"Driver"`
	Scope      string                          `json:"Scope"`
	Internal   bool                            `json:"Internal"`
	EnableIPv6 bool                            `json:"EnableIPv6"`
	Containers map[string]NetworkContainerInfo `json:"Containers"`
}

type NetworkSummary struct {
	ID         string            `json:"id"`
	ShortID    string            `json:"short_id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	EnableIPv6 bool              `json:"enable_ipv6"`
	Containers map[string]string `json:"containers"` // ContainerID -> Name (or IP)
	Engine     string            `json:"engine"`     // "podman" or "docker"
}

func (c *Client) ListNetworks() ([]NetworkSummary, error) {
	// Pre-fetch container names map
	containerNames := make(map[string]string)
	if ctrsList, errCtrs := c.ListContainers(); errCtrs == nil {
		for _, ctr := range ctrsList {
			containerNames[ctr.ID] = ctr.Name
			containerNames[ctr.ShortID] = ctr.Name
		}
	}

	var result []NetworkSummary
	seenIDs := make(map[string]bool)

	for _, sc := range c.allClients {
		body, code, err := sc.Get("/networks")
		if err != nil || code != 200 {
			continue
		}

		var rawList []RawNetwork
		if err := json.Unmarshal(body, &rawList); err != nil {
			continue
		}

		engineName := sc.Engine()

		for _, r := range rawList {
			key := engineName + ":" + r.ID
			if seenIDs[key] {
				continue
			}
			seenIDs[key] = true

			shortID := r.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			ctrs := make(map[string]string)
			for cid, cinfo := range r.Containers {
				cShortID := cid
				if len(cShortID) > 12 {
					cShortID = cShortID[:12]
				}

				label := ""
				if actualName, ok := containerNames[cid]; ok && actualName != "" {
					label = actualName
				} else if actualName, ok := containerNames[cShortID]; ok && actualName != "" {
					label = actualName
				} else if cinfo.Name != "" && cinfo.Name != "podman" {
					label = cinfo.Name
				} else {
					label = cShortID
				}

				if cinfo.IPv4Address != "" {
					label += fmt.Sprintf(" (%s)", cinfo.IPv4Address)
				}
				ctrs[cShortID] = label
			}

			result = append(result, NetworkSummary{
				ID:         r.ID,
				ShortID:    shortID,
				Name:       r.Name,
				Driver:     r.Driver,
				Scope:      r.Scope,
				Internal:   r.Internal,
				EnableIPv6: r.EnableIPv6,
				Containers: ctrs,
				Engine:     engineName,
			})
		}
	}

	return result, nil
}

func (c *Client) CreateNetwork(name, driver string) error {
	if driver == "" {
		driver = "bridge"
	}
	reqObj := map[string]interface{}{
		"Name":   name,
		"Driver": driver,
	}
	reqBytes, _ := json.Marshal(reqObj)

	var lastErr error
	for _, sc := range c.allClients {
		body, code, err := sc.Post("/networks/create", reqBytes)
		if err == nil && (code == 201 || code == 200) {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("create network failed status %d: %s", code, string(body))
		}
	}
	return lastErr
}

func (c *Client) RemoveNetwork(id string) error {
	path := fmt.Sprintf("/networks/%s", id)
	var lastErr error
	for _, sc := range c.allClients {
		body, code, err := sc.Delete(path)
		if err == nil && (code == 204 || code == 200) {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("remove network failed status %d: %s", code, string(body))
		}
	}
	return lastErr
}
