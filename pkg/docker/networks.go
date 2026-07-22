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
}

func (c *Client) ListNetworks() ([]NetworkSummary, error) {
	body, code, err := c.Get("/networks")
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("docker networks error status %d: %s", code, string(body))
	}

	var rawList []RawNetwork
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, err
	}

	var result []NetworkSummary
	for _, r := range rawList {
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
			label := cinfo.Name
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
		})
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

	body, code, err := c.Post("/networks/create", reqBytes)
	if err != nil {
		return err
	}
	if code != 201 && code != 200 {
		return fmt.Errorf("create network failed (status %d): %s", code, string(body))
	}
	return nil
}

func (c *Client) RemoveNetwork(id string) error {
	path := fmt.Sprintf("/networks/%s", id)
	body, code, err := c.Delete(path)
	if err != nil {
		return err
	}
	if code != 204 && code != 200 {
		return fmt.Errorf("remove network failed (status %d): %s", code, string(body))
	}
	return nil
}
