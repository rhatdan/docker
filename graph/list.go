package graph

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/parsers/filters"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/utils"
)

var acceptedImageFilterTags = map[string]struct{}{
	"dangling": {},
	"label":    {},
}

type ImagesConfig struct {
	Filters string
	Filter  string
	All     bool
}

type ByCreated []*types.Image

func (r ByCreated) Len() int           { return len(r) }
func (r ByCreated) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ByCreated) Less(i, j int) bool { return r[i].Created < r[j].Created }

type byTagName []*types.RepositoryTag

func (r byTagName) Len() int           { return len(r) }
func (r byTagName) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byTagName) Less(i, j int) bool { return r[i].Tag < r[j].Tag }

type byAPIVersion []registry.APIEndpoint

func (r byAPIVersion) Len() int      { return len(r) }
func (r byAPIVersion) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byAPIVersion) Less(i, j int) bool {
	if r[i].Version < r[j].Version {
		return true
	}
	if r[i].Version == r[j].Version && strings.HasPrefix(r[i].URL, "https://") && !strings.HasPrefix(r[j].URL, "https://") {
		return true
	}
	return false
}

// RemoteTagsConfig allows to specify transport paramater for remote ta listing.
type RemoteTagsConfig struct {
	MetaHeaders map[string][]string
	AuthConfig  *cliconfig.AuthConfig
}

// TagLister allows to list tags of remote repository.
type TagLister interface {
	ListTags() (tagList []*types.RepositoryTag, fallback bool, err error)
}

// NewTagLister creates a specific tag lister for given endpoint.
func NewTagLister(s *TagStore, endpoint registry.APIEndpoint, repoInfo *registry.RepositoryInfo, config *RemoteTagsConfig) (TagLister, error) {
	switch endpoint.Version {
	case registry.APIVersion2:
		return &v2TagLister{
			TagStore: s,
			endpoint: endpoint,
			config:   config,
			repoInfo: repoInfo,
		}, nil
	case registry.APIVersion1:
		return &v1TagLister{
			TagStore: s,
			endpoint: endpoint,
			config:   config,
			repoInfo: repoInfo,
		}, nil
	}
	return nil, fmt.Errorf("unknown version %d for registry %s", endpoint.Version, endpoint.URL)
}

func (s *TagStore) Images(config *ImagesConfig) ([]*types.Image, error) {
	var (
		allImages  map[string]*image.Image
		err        error
		filtTagged = true
		filtLabel  = false
	)

	imageFilters, err := filters.FromParam(config.Filters)
	if err != nil {
		return nil, err
	}
	for name := range imageFilters {
		if _, ok := acceptedImageFilterTags[name]; !ok {
			return nil, fmt.Errorf("Invalid filter '%s'", name)
		}
	}

	if i, ok := imageFilters["dangling"]; ok {
		for _, value := range i {
			if strings.ToLower(value) == "true" {
				filtTagged = false
			}
		}
	}

	_, filtLabel = imageFilters["label"]

	if config.All && filtTagged {
		allImages = s.graph.Map()
	} else {
		allImages = s.graph.Heads()
	}

	// try to match filter against all repositories from additional registries
	// when dealing with short name
	repoNameFilters := make([]string, 1, 1+len(registry.RegistryList))
	repoNameFilters[0] = config.Filter
	if strings.IndexByte(config.Filter, '/') == -1 {
		for _, r := range registry.RegistryList {
			repoNameFilters = append(repoNameFilters, r+"/"+config.Filter)
		}
	}

	lookup := make(map[string]*types.Image)
	s.Lock()
	for repoName, repository := range s.Repositories {
		if repoNameFilters[0] != "" {
			match := false
			for _, filter := range repoNameFilters {
				if match, _ = path.Match(filter, repoName); match {
					break
				}
			}
			if !match {
				continue
			}
		}
		for ref, id := range repository {
			imgRef := utils.ImageReference(repoName, ref)
			image, err := s.graph.Get(id)
			if err != nil {
				logrus.Warnf("couldn't load %s from %s: %s", id, imgRef, err)
				continue
			}

			if lImage, exists := lookup[id]; exists {
				if filtTagged {
					if utils.DigestReference(ref) {
						lImage.RepoDigests = append(lImage.RepoDigests, imgRef)
					} else { // Tag Ref.
						lImage.RepoTags = append(lImage.RepoTags, imgRef)
					}
				}
			} else {
				// get the boolean list for if only the untagged images are requested
				delete(allImages, id)
				if !imageFilters.MatchKVList("label", image.ContainerConfig.Labels) {
					continue
				}
				if filtTagged {
					newImage := new(types.Image)
					newImage.ParentId = image.Parent
					newImage.ID = image.ID
					newImage.Created = int(image.Created.Unix())
					newImage.Size = int(image.Size)
					newImage.VirtualSize = int(s.graph.GetParentsSize(image, 0) + image.Size)
					newImage.Labels = image.ContainerConfig.Labels

					if utils.DigestReference(ref) {
						newImage.RepoTags = []string{}
						newImage.RepoDigests = []string{imgRef}
					} else {
						newImage.RepoTags = []string{imgRef}
						newImage.RepoDigests = []string{}
					}

					lookup[id] = newImage
				}
			}

		}
	}
	s.Unlock()

	images := []*types.Image{}
	for _, value := range lookup {
		images = append(images, value)
	}

	// Display images which aren't part of a repository/tag
	if config.Filter == "" || filtLabel {
		for _, image := range allImages {
			if !imageFilters.MatchKVList("label", image.ContainerConfig.Labels) {
				continue
			}
			newImage := new(types.Image)
			newImage.ParentId = image.Parent
			newImage.RepoTags = []string{"<none>:<none>"}
			newImage.RepoDigests = []string{"<none>@<none>"}
			newImage.ID = image.ID
			newImage.Created = int(image.Created.Unix())
			newImage.Size = int(image.Size)
			newImage.VirtualSize = int(s.graph.GetParentsSize(image, 0) + image.Size)
			newImage.Labels = image.ContainerConfig.Labels

			images = append(images, newImage)
		}
	}

	sort.Sort(sort.Reverse(ByCreated(images)))

	return images, nil
}

// Tags returns a tag list for given local repository.
func (s *TagStore) Tags(name string) (*types.RepositoryTagList, error) {
	var tagList *types.RepositoryTagList

	// Resolve the Repository name from fqn to RepositoryInfo
	repos := s.getRepositoryList(name)
	if len(repos) < 1 {
		return nil, fmt.Errorf("no such repository %q", name)
	}

	for repoName, repo := range repos[0] {
		tagList = &types.RepositoryTagList{
			Name:    repoName,
			TagList: make([]*types.RepositoryTag, 0, len(repo)),
		}

		for ref, id := range repo {
			tagList.TagList = append(tagList.TagList, &types.RepositoryTag{
				Tag:     ref,
				ImageID: id,
			})
		}
	}

	sort.Sort(byTagName(tagList.TagList))
	return tagList, nil
}

// RemoteTags fetches a tag list from remote repository
func (s *TagStore) RemoteTags(name string, config *RemoteTagsConfig) (*types.RepositoryTagList, error) {
	var (
		tagList *types.RepositoryTagList
		err     error
	)
	// Unless the index name is specified, iterate over all registries until
	// the matching image is found.
	if registry.RepositoryNameHasIndex(name) {
		return s.getRemoteTagList(name, config)
	}
	if len(registry.RegistryList) == 0 {
		return nil, fmt.Errorf("No configured registry to pull from.")
	}
	for _, r := range registry.RegistryList {
		// Prepend the index name to the image name.
		if tagList, err = s.getRemoteTagList(fmt.Sprintf("%s/%s", r, name), config); err == nil {
			return tagList, nil
		}
	}
	return tagList, err
}

func (s *TagStore) getRemoteTagList(name string, config *RemoteTagsConfig) (*types.RepositoryTagList, error) {
	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := s.registryService.ResolveRepository(name)
	if err != nil {
		return nil, err
	}

	if err := validateRepoName(repoInfo.LocalName); err != nil {
		return nil, err
	}

	endpoints, err := s.registryService.LookupPullEndpoints(repoInfo.CanonicalName)
	if err != nil {
		return nil, err
	}
	// Prefer v1 versions which provide also image ids
	sort.Sort(byAPIVersion(endpoints))

	var (
		lastErr error
		// discardNoSupportErrors is used to track whether an endpoint encountered an error of type registry.ErrNoSupport
		// By default it is false, which means that if a ErrNoSupport error is encountered, it will be saved in lastErr.
		// As soon as another kind of error is encountered, discardNoSupportErrors is set to true, avoiding the saving of
		// any subsequent ErrNoSupport errors in lastErr.
		// It's needed for pull-by-digest on v1 endpoints: if there are only v1 endpoints configured, the error should be
		// returned and displayed, but if there was a v2 endpoint which supports pull-by-digest, then the last relevant
		// error is the ones from v2 endpoints not v1.
		discardNoSupportErrors bool
		tagList                = &types.RepositoryTagList{Name: repoInfo.CanonicalName}
	)
	for _, endpoint := range endpoints {
		logrus.Debugf("Trying to fetch tag list of %s repository from %s %s", repoInfo.CanonicalName, endpoint.URL, endpoint.Version)
		fallback := false

		if !endpoint.Mirror && (endpoint.Official || endpoint.Version == registry.APIVersion2) {
			if repoInfo.Official {
				s.trustService.UpdateBase()
			}
		}

		tagLister, err := NewTagLister(s, endpoint, repoInfo, config)
		if err != nil {
			lastErr = err
			continue
		}
		tagList.TagList, fallback, err = tagLister.ListTags()
		if err != nil {
			// We're querying v1 registries first. Let's ignore errors until
			// the first v2 registry.
			if fallback || endpoint.Version == registry.APIVersion1 {
				if _, ok := err.(registry.ErrNoSupport); !ok {
					// Because we found an error that's not ErrNoSupport, discard all subsequent ErrNoSupport errors.
					discardNoSupportErrors = true
					// save the current error
					lastErr = err
				} else if !discardNoSupportErrors {
					// Save the ErrNoSupport error, because it's either the first error or all encountered errors
					// were also ErrNoSupport errors.
					lastErr = err
				}
				continue
			}
			logrus.Debugf("Not continuing with error: %v", err)
			return nil, err
		}

		sort.Sort(byTagName(tagList.TagList))
		return tagList, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no endpoints found for %s", repoInfo.Index.Name)
	}
	return nil, lastErr
}
