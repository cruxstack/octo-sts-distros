// Copyright 2025 CruxStack
// SPDX-License-Identifier: MIT

package installer

// Manifest represents the GitHub App manifest structure.
// See https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
type Manifest struct {
	// Name is the display name of the GitHub App.
	// If empty, the user will be prompted to enter a name.
	Name string `json:"name,omitempty"`

	// URL is the homepage URL of the GitHub App (shown in the app settings).
	URL string `json:"url"`

	// HookAttributes configures the webhook for the GitHub App.
	// If URL is empty, the webhook will be disabled.
	HookAttributes HookAttributes `json:"hook_attributes"`

	// RedirectURL is automatically set by the installer based on the request.
	// You do not need to set this field.
	RedirectURL string `json:"redirect_url"`

	// Public determines whether the app can be installed by other users/orgs.
	// Set to false for internal apps.
	Public bool `json:"public"`

	// DefaultPerms is the set of permissions the app requests.
	// Keys are permission names (e.g., "contents", "pull_requests").
	// Values are "read" or "write".
	DefaultPerms map[string]string `json:"default_permissions"`

	// DefaultEvents is the list of webhook events the app subscribes to.
	// E.g., ["pull_request", "push"]
	DefaultEvents []string `json:"default_events"`
}

// HookAttributes configures the webhook for the GitHub App.
type HookAttributes struct {
	// URL is the webhook endpoint URL.
	// If empty, the installer will auto-derive it from the request.
	URL string `json:"url"`

	// Active determines whether the webhook is enabled.
	Active bool `json:"active"`
}

// Clone returns a deep copy of the manifest.
func (m *Manifest) Clone() *Manifest {
	if m == nil {
		return nil
	}

	clone := &Manifest{
		Name:           m.Name,
		URL:            m.URL,
		RedirectURL:    m.RedirectURL,
		Public:         m.Public,
		HookAttributes: m.HookAttributes,
	}

	if m.DefaultPerms != nil {
		clone.DefaultPerms = make(map[string]string, len(m.DefaultPerms))
		for k, v := range m.DefaultPerms {
			clone.DefaultPerms[k] = v
		}
	}

	if m.DefaultEvents != nil {
		clone.DefaultEvents = make([]string, len(m.DefaultEvents))
		copy(clone.DefaultEvents, m.DefaultEvents)
	}

	return clone
}
