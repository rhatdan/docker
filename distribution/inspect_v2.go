package distribution

import (
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/docker/image"
	"github.com/docker/docker/image/v1"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	"github.com/docker/engine-api/types"
	"golang.org/x/net/context"
)

type v2ManifestFetcher struct {
	endpoint registry.APIEndpoint
	config   *InspectConfig
	repoInfo *registry.RepositoryInfo
	repo     distribution.Repository
	// confirmedV2 is set to true if we confirm we're talking to a v2
	// registry. This is used to limit fallbacks to the v1 protocol.
	confirmedV2 bool
}

func (mf *v2ManifestFetcher) Fetch(ctx context.Context, ref reference.Named) (imgInspect *types.RemoteImageInspect, err error) {
	mf.repo, mf.confirmedV2, err = NewV2Repository(ctx, mf.repoInfo, mf.endpoint, mf.config.MetaHeaders, mf.config.AuthConfig, "pull")
	if err != nil {
		logrus.Debugf("Error getting v2 registry: %v", err)
		return nil, fallbackError{err: err, confirmedV2: mf.confirmedV2}
	}

	imgInspect, err = mf.fetchWithRepository(ctx, ref)
	if err != nil {
		switch t := err.(type) {
		case errcode.Errors:
			if len(t) == 1 {
				err = t[0]
			}
		}
		if registry.ContinueOnError(err) {
			logrus.Debugf("Error trying v2 registry: %v", err)
			err = fallbackError{err: err, confirmedV2: mf.confirmedV2}
		}
	}
	return
}

func (mf *v2ManifestFetcher) fetchWithRepository(ctx context.Context, ref reference.Named) (*types.RemoteImageInspect, error) {
	var (
		exists             bool
		dgst               digest.Digest
		err                error
		img                *image.Image
		unverifiedManifest *schema1.SignedManifest
		tag                string
		tagOrDigest        string
	)

	manSvc, err := mf.repo.Manifests(ctx)
	if err != nil {
		return nil, err
	}
	if digested, isCanonical := ref.(reference.Canonical); isCanonical {
		exists, err = manSvc.Exists(digested.Digest())
		if err == nil && !exists {
			return nil, fmt.Errorf("Digest %q does not exist in remote repository %s", digested.Digest().String(), mf.repoInfo.FullName())
		}
		if exists {
			unverifiedManifest, err = manSvc.Get(digested.Digest())
		}
		tagOrDigest = digested.Digest().String()

	} else {
		if tagged, isTagged := ref.(reference.NamedTagged); isTagged {
			tag = tagged.Tag()

		} else {
			tagList, err := manSvc.Tags()
			if err != nil {
				return nil, err
			}
			for _, t := range tagList {
				if t == reference.DefaultTag {
					tag = reference.DefaultTag
				}
			}
			if tag == "" && len(tagList) > 0 {
				tag = tagList[0]
			}
			if tag == "" {
				return nil, fmt.Errorf("No tags available for remote repository %s", mf.repoInfo.FullName())
			}
		}

		unverifiedManifest, err = manSvc.GetByTag(tag)
		tagOrDigest = tag
	}

	if err != nil {
		return nil, err
	}
	if unverifiedManifest == nil {
		return nil, fmt.Errorf("image manifest does not exist for tag or digest %q", tagOrDigest)
	}

	var verifiedManifest *schema1.Manifest
	verifiedManifest, err = verifyManifest(unverifiedManifest, ref)
	if err != nil {
		return nil, err
	}

	rootFS := image.NewRootFS()

	// remove duplicate layers and check parent chain validity
	err = fixManifestLayers(verifiedManifest)
	if err != nil {
		return nil, err
	}

	// Image history converted to the new format
	var history []image.History

	// Note that the order of this loop is in the direction of bottom-most
	// to top-most, so that the downloads slice gets ordered correctly.
	for i := len(verifiedManifest.FSLayers) - 1; i >= 0; i-- {
		var throwAway struct {
			ThrowAway bool `json:"throwaway,omitempty"`
		}
		if err := json.Unmarshal([]byte(verifiedManifest.History[i].V1Compatibility), &throwAway); err != nil {
			return nil, err
		}

		h, err := v1.HistoryFromConfig([]byte(verifiedManifest.History[i].V1Compatibility), throwAway.ThrowAway)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	configRaw, err := v1.MakeRawConfigFromV1Config([]byte(unverifiedManifest.History[0].V1Compatibility), rootFS, history)
	if err != nil {
		return nil, err
	}

	config, err := json.Marshal(configRaw)
	if err != nil {
		return nil, err
	}

	dgst, _, err = digestFromManifest(unverifiedManifest, mf.repoInfo)
	if err != nil {
		return nil, err
	}

	img, err = image.NewFromJSON(config)
	if err != nil {
		return nil, err
	}

	return makeRemoteImageInspect(mf.repoInfo, img, tag, dgst), nil
}
