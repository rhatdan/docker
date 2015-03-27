package registry

import (
	"github.com/docker/distribution/reference"
	registrytypes "github.com/docker/docker/api/types/registry"
)

// SearchResult describes a search result returned from a registry
type SearchResult struct {
	// StarCount indicates the number of stars this repository has
	StarCount int `json:"star_count"`
	// IsOfficial indicates whether the result is an official repository or not
	IsOfficial bool `json:"is_official"`
	// Name is the name of the repository
	Name string `json:"name"`
	// IsOfficial indicates whether the result is trusted
	IsTrusted bool `json:"is_trusted"`
	// IsAutomated indicates whether the result is automated
	IsAutomated bool `json:"is_automated"`
	// Description is a textual description of the repository
	Description string `json:"description"`
}

// SearchResults lists a collection search results returned from a registry
type SearchResults struct {
	// Query contains the query string that generated the search results
	Query string `json:"query"`
	// NumResults indicates the number of results the query returned
	NumResults int `json:"num_results"`
	// Results is a slice containing the actual results for the search
	Results []SearchResult `json:"results"`
}

// RepositoryData tracks the image list, list of endpoints, and list of tokens
// for a repository
type RepositoryData struct {
	// ImgList is a list of images in the repository
	ImgList map[string]*ImgData
	// Endpoints is a list of endpoints returned in X-Docker-Endpoints
	Endpoints []string
	// Tokens is currently unused (remove it?)
	Tokens []string
}

// ImgData is used to transfer image checksums to and from the registry
type ImgData struct {
	// ID is an opaque string that identifies the image
	ID              string `json:"id"`
	Checksum        string `json:"checksum,omitempty"`
	ChecksumPayload string `json:"-"`
	Tag             string `json:",omitempty"`
}

// PingResult contains the information returned when pinging a registry. It
// indicates the registry's version and whether the registry claims to be a
// standalone registry.
type PingResult struct {
	// Version is the registry version supplied by the registry in a HTTP
	// header
	Version string `json:"version"`
	// Standalone is set to true if the registry indicates it is a
	// standalone registry in the X-Docker-Registry-Standalone
	// header
	Standalone bool `json:"standalone"`
}

// APIVersion is an integral representation of an API version (presently
// either 1 or 2)
type APIVersion int

func (av APIVersion) String() string {
	return apiVersions[av]
}

var apiVersions = map[APIVersion]string{
	1: "v1",
	2: "v2",
}

// API Version identifiers.
const (
	APIVersionUnknown = iota
	APIVersion1
	APIVersion2
)

// RepositoryInfo describes a repository
type RepositoryInfo struct {
	// Index points to registry information
	Index *registrytypes.IndexInfo
	// RemoteName is the remote name of the repository, such as
	// "library/ubuntu-12.04-base"
	RemoteName reference.Named
	// LocalName is the local name of the repository, such as
	// "ubuntu-12.04-base"
	LocalName reference.Named
	// CanonicalName is the canonical name of the repository, such as
	// "docker.io/library/ubuntu-12.04-base"
	CanonicalName reference.Named
	// Official indicates whether the repository is considered official.
	// If the registry is official, and the normalized name does not
	// contain a '/' (e.g. "foo"), then it is considered an official repo.
	Official bool
}

// SearchResultExt describes a search result returned from a registry extended for
// IndexName and RegistryName.
// GET "/images/search"
type SearchResultExt struct {
	// IndexName is a name of index server providing the data below.
	IndexName string `json:"index_name"`
	// RegistryName is a registry name part of the Name. If the Name is not fully
	// qualified, it will be set to IndexName.
	RegistryName string `json:"registry_name"`
	// StarCount indicates the number of stars this repository has
	StarCount int `json:"star_count"`
	// IsOfficial indicates whether the result is an official repository or not
	IsOfficial bool `json:"is_official"`
	// Name is the name of the repository
	Name string `json:"name"`
	// IsOfficial indicates whether the result is trusted
	IsTrusted bool `json:"is_trusted"`
	// IsAutomated indicates whether the result is automated
	IsAutomated bool `json:"is_automated"`
	// Description is a textual description of the repository
	Description string `json:"description"`
}
