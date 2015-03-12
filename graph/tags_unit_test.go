package graph

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/docker/docker/daemon/graphdriver"
	_ "github.com/docker/docker/daemon/graphdriver/vfs" // import the vfs driver so it is used in the tests
	"github.com/docker/docker/image"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/utils"
	"github.com/docker/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	testLocalImageName      = "myapp"
	testLocalImageID        = "1a2d3c4d4e5fa2d2a21acea242a5e2345d3aefc3e7dfa2a2a2a21a2a2ad2d234"
	testLocalImageIDShort   = "1a2d3c4d4e5f"
	testPrivateIndexName    = "127.0.0.1:8000"
	testPrivateRemoteName   = "privateapp"
	testPrivateImageName    = testPrivateIndexName + "/" + testPrivateRemoteName
	testPrivateImageID      = "5bc255f8699e4ee89ac4469266c3d11515da88fdcbde45d7b069b636ff4efd81"
	testPrivateImageIDShort = "5bc255f8699e"
)

func fakeTar() (io.Reader, error) {
	uid := os.Getuid()
	gid := os.Getgid()

	content := []byte("Hello world!\n")
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for _, name := range []string{"/etc/postgres/postgres.conf", "/etc/passwd", "/var/log/postgres/postgres.conf"} {
		hdr := new(tar.Header)

		// Leaving these fields blank requires root privileges
		hdr.Uid = uid
		hdr.Gid = gid

		hdr.Size = int64(len(content))
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		tw.Write([]byte(content))
	}
	tw.Close()
	return buf, nil
}

func mkTestTagStore(root string, t *testing.T) *TagStore {
	driver, err := graphdriver.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGraph(root, driver)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTagStore(path.Join(root, "tags"), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	localArchive, err := fakeTar()
	if err != nil {
		t.Fatal(err)
	}
	img := &image.Image{ID: testLocalImageID}
	if err := graph.Register(img, localArchive); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(testLocalImageName, "", testLocalImageID, false, true); err != nil {
		t.Fatal(err)
	}
	privateArchive, err := fakeTar()
	if err != nil {
		t.Fatal(err)
	}
	img = &image.Image{ID: testPrivateImageID}
	if err := graph.Register(img, privateArchive); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(testPrivateImageName, "", testPrivateImageID, false, true); err != nil {
		t.Fatal(err)
	}
	return store
}

func imageCount(s *TagStore) int {
	cnt := 0
	for _, repo := range s.Repositories {
		cnt += len(repo)
	}
	return cnt
}

func logStoreContent(t *testing.T, s *TagStore, caseNumber int) {
	prefix := ""
	if caseNumber >= 0 {
		prefix = fmt.Sprintf("[case#%d] ", caseNumber)
	}
	t.Logf("%sstore.Repositories content:", prefix, caseNumber)
	for name, repo := range s.Repositories {
		t.Logf("%s  %s :", prefix, caseNumber, name)
		for tag, id := range repo {
			t.Logf("%s    %s : %s", prefix, caseNumber, tag, id)
		}
	}
}

func TestLookupImage(t *testing.T) {
	tmp, err := utils.TestDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	store := mkTestTagStore(tmp, t)
	defer store.graph.driver.Cleanup()

	localLookups := []string{
		testLocalImageID,
		testLocalImageIDShort,
		testLocalImageName + ":" + testLocalImageID,
		testLocalImageName + ":" + testLocalImageIDShort,
		testLocalImageName,
		testLocalImageName + ":" + DEFAULTTAG,
	}

	privateLookups := []string{
		testPrivateImageID,
		testPrivateImageIDShort,
		testPrivateImageName + ":" + testPrivateImageID,
		testPrivateImageName + ":" + testPrivateImageIDShort,
		testPrivateImageName,
		testPrivateImageName + ":" + DEFAULTTAG,
		testPrivateRemoteName + ":" + testPrivateImageID,
		testPrivateRemoteName + ":" + testPrivateImageIDShort,
		testPrivateRemoteName,
		testPrivateRemoteName + ":" + DEFAULTTAG,
	}

	invalidLookups := []string{
		testLocalImageName + ":" + "fail",
		"docker.io/" + testPrivateRemoteName,
		testPrivateIndexName + "/" + testLocalImageName,
		"fail:fail",
		// these should fail, because testLocalImageName isn't fully qualified
		"docker.io/" + testLocalImageName,
		"docker.io/" + testLocalImageName + ":" + DEFAULTTAG,
		"index.docker.io/" + testLocalImageName,
		"index.docker.io/" + testLocalImageName + ":" + DEFAULTTAG,
		"library/" + testLocalImageName,
		"library/" + testLocalImageName + ":" + DEFAULTTAG,
		"docker.io/library/" + testLocalImageName,
		"docker.io/library/" + testLocalImageName + ":" + DEFAULTTAG,
		"index.docker.io/library/" + testLocalImageName,
		"index.docker.io/library/" + testLocalImageName + ":" + DEFAULTTAG,
	}

	runCases := func(imageID string, cases []string, valid bool) {
		for _, name := range cases {
			if valid {
				if img, err := store.LookupImage(name); err != nil {
					t.Errorf("Error looking up %s: %s", name, err)
				} else if img == nil {
					t.Errorf("Expected 1 image, none found: %s", name)
				} else if imageID != "" && img.ID != imageID {
					t.Errorf("Expected ID '%s' found '%s'", imageID, img.ID)
				}
			} else {
				if img, err := store.LookupImage(name); err == nil {
					t.Errorf("Expected error, none found: %s", name)
				} else if img != nil {
					t.Errorf("Expected 0 image, 1 found: %s", name)
				}
			}
		}
	}

	runCases(testLocalImageID, localLookups, true)
	runCases(testPrivateImageID, privateLookups, true)
	runCases("", invalidLookups, false)

	// now make local image fully qualified (`docker.io` will be prepended)
	store.Set(testLocalImageName, "", testLocalImageID, false, false)
	store.Delete(testLocalImageName, "latest")

	if imageCount(store) != 2 {
		t.Fatalf("Expected two images in tag store, not %d.", imageCount(store))
	}
	corrupted := false
	for _, repoName := range []string{"docker.io/" + testLocalImageName, testPrivateImageName} {
		if repo, exists := store.Repositories[repoName]; !exists {
			corrupted = true
			break
		} else if _, exists := repo["latest"]; !exists {
			corrupted = true
			break
		}
	}
	if corrupted {
		logStoreContent(t, store, -1)
		t.Fatalf("TagStore got corrupted!")
	}

	// and retest lookups of local image - now prefixed with `docker.io`
	localLookups = []string{
		testLocalImageID,
		testLocalImageIDShort,
		testLocalImageName + ":" + testLocalImageID,
		testLocalImageName + ":" + testLocalImageIDShort,
		testLocalImageName,
		testLocalImageName + ":" + DEFAULTTAG,
		"docker.io/" + testLocalImageName,
		"docker.io/" + testLocalImageName + ":" + DEFAULTTAG,
		"index.docker.io/" + testLocalImageName,
		"index.docker.io/" + testLocalImageName + ":" + DEFAULTTAG,
		"library/" + testLocalImageName,
		"library/" + testLocalImageName + ":" + DEFAULTTAG,
		"docker.io/library/" + testLocalImageName,
		"docker.io/library/" + testLocalImageName + ":" + DEFAULTTAG,
		"index.docker.io/library/" + testLocalImageName,
		"index.docker.io/library/" + testLocalImageName + ":" + DEFAULTTAG,
	}

	invalidLookups = []string{
		testLocalImageName + ":" + "fail",
		"docker.io/" + testPrivateRemoteName,
		testPrivateIndexName + "/" + testLocalImageName,
		"fail:fail",
	}

	runCases(testLocalImageID, localLookups, true)
	runCases(testPrivateImageID, privateLookups, true)
	runCases("", invalidLookups, false)
}

func TestValidTagName(t *testing.T) {
	validTags := []string{"9", "foo", "foo-test", "bar.baz.boo"}
	for _, tag := range validTags {
		if err := ValidateTagName(tag); err != nil {
			t.Errorf("'%s' should've been a valid tag", tag)
		}
	}
}

func TestInvalidTagName(t *testing.T) {
	validTags := []string{"-9", ".foo", "-test", ".", "-"}
	for _, tag := range validTags {
		if err := ValidateTagName(tag); err == nil {
			t.Errorf("'%s' shouldn't have been a valid tag", tag)
		}
	}
}

type setTagCase struct {
	imageID        string
	dest           string
	destTag        string
	preserveName   bool
	shallSucceed   bool
	expectedResult string
}

var setTagCases = []setTagCase{
	setTagCase{testLocalImageID, testLocalImageName, "", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testLocalImageID, testLocalImageName, "", true, false, ""},
	setTagCase{testLocalImageID, testLocalImageName, "latest", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testLocalImageID, testLocalImageName, "latest", true, false, ""},
	setTagCase{testLocalImageID, testLocalImageName, "foo", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testLocalImageID, testLocalImageName, "foo", true, true, testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "", false, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "", true, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "latest", false, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "latest", true, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "foo", false, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/" + testLocalImageName, "foo", true, true, "myrepo.io/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateImageName, "", false, false, ""},
	setTagCase{testLocalImageID, testPrivateImageName, "", true, false, ""},
	setTagCase{testLocalImageID, testPrivateImageName, "latest", false, false, ""},
	setTagCase{testLocalImageID, testPrivateImageName, "latest", true, false, ""},
	setTagCase{testLocalImageID, testPrivateImageName, "foo", false, true, testPrivateImageName},
	setTagCase{testLocalImageID, testPrivateImageName, "foo", true, true, testPrivateImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "", false, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "", true, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "latest", false, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "latest", true, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "foo", false, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, testPrivateIndexName + "/" + testLocalImageName, "foo", true, true, testPrivateIndexName + "/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "", false, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "", true, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "latest", false, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "latest", true, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "foo", false, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testLocalImageID, "myrepo.io/library/" + testLocalImageName, "foo", true, true, "myrepo.io/library/" + testLocalImageName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "", true, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "latest", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "latest", true, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "foo", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/" + testPrivateRemoteName, "foo", true, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "", true, true, "docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "latest", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "latest", true, true, "docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "foo", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "docker.io/library/" + testPrivateRemoteName, "foo", true, true, "docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "", true, true, "index.docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "latest", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "latest", true, true, "index.docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "foo", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/" + testPrivateRemoteName, "foo", true, true, "index.docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "", true, true, "index.docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "latest", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "latest", true, true, "index.docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "foo", false, true, "docker.io/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, "index.docker.io/library/" + testPrivateRemoteName, "foo", true, true, "index.docker.io/library/" + testPrivateRemoteName},
	setTagCase{testPrivateImageID, testLocalImageName, "", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testPrivateImageID, testLocalImageName, "", true, false, ""},
	setTagCase{testPrivateImageID, testLocalImageName, "latest", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testPrivateImageID, testLocalImageName, "latest", true, false, ""},
	setTagCase{testPrivateImageID, testLocalImageName, "foo", false, true, "docker.io/" + testLocalImageName},
	setTagCase{testPrivateImageID, testLocalImageName, "foo", true, true, testLocalImageName},
}

func runSetTagCases(t *testing.T, store *TagStore, additionalRegistry string) {
	localImages := map[string]string{
		testLocalImageID:   testLocalImageName,
		testPrivateImageID: testPrivateImageName,
	}
	for i, testCase := range setTagCases {
		for _, source := range []string{testCase.imageID, localImages[testCase.imageID]} {
			for _, sourceTag := range []string{"", "latest"} {
				if source == testCase.imageID && sourceTag != "" {
					continue
				}
				taggedSource := source
				if sourceTag != "" {
					taggedSource = source + ":" + sourceTag
				}
				dest := testCase.dest
				expectedResult := testCase.expectedResult
				if !registry.RepositoryNameHasIndex(testCase.dest) && !testCase.preserveName && additionalRegistry != "" {
					_, remoteName := registry.SplitReposName(expectedResult, false)
					expectedResult = additionalRegistry + "/" + remoteName
				}
				if testCase.destTag != "" {
					dest = testCase.dest + ":" + testCase.destTag
					expectedResult = expectedResult + ":" + testCase.destTag
				}

				err := store.Set(testCase.dest, testCase.destTag, taggedSource, false, testCase.preserveName)
				if err == nil && !testCase.shallSucceed {
					logStoreContent(t, store, i)
					t.Fatalf("[case#%d] Tagging of %q as %q should have failed.", i, taggedSource, dest)
				}
				if err != nil && testCase.shallSucceed {
					logStoreContent(t, store, i)
					t.Errorf("[case#%d] Tagging of %q as %q should have succeeded: %v.", i, taggedSource, dest, err)
					continue
				}
				if err != nil {
					continue
				}

				if imageCount(store) != 3 {
					logStoreContent(t, store, i)
					t.Fatalf("[case#%d] Expected 3 images in TagStore, not %d.", i, imageCount(store))
				}

				if img, err := store.LookupImage(dest); err != nil {
					t.Errorf("[case#%d] Error looking up %q: %s", i, dest, err)
				} else if img == nil {
					t.Errorf("[case#%d] Expected 1 image, none found.", i)
				}

				if img, err := store.LookupImage(expectedResult); err != nil {
					t.Errorf("[case#%d] Error looking up %q: %s", i, expectedResult, err)
				} else if img == nil {
					t.Errorf("[case#%d] Expected 1 image, none found.", i)
				} else if img.ID != testCase.imageID {
					t.Errorf("[case#%d] Expected ID %q found %q", i, testCase.imageID, img.ID)
				}

				toDelete := expectedResult
				if strings.HasSuffix(expectedResult, ":"+testCase.destTag) {
					toDelete = expectedResult[:len(expectedResult)-len(":"+testCase.destTag)]
				}
				if ok, err := store.Delete(toDelete, testCase.destTag); err != nil || !ok {
					logStoreContent(t, store, i)
					t.Fatalf("[case#%d] Deletion of %q should have succeeded: %v", i, expectedResult, err)
				}
				if imageCount(store) != 2 {
					logStoreContent(t, store, i)
					t.Fatalf("[case#%d] Expected 2 repositories in TagStore, not %d.", i, imageCount(store))
				}
				corrupted := false
				for _, repoName := range []string{testLocalImageName, testPrivateImageName} {
					if repo, exists := store.Repositories[repoName]; !exists {
						corrupted = true
						break
					} else if _, exists := repo["latest"]; !exists {
						corrupted = true
						break
					}
				}
				if corrupted {
					logStoreContent(t, store, i)
					t.Fatalf("[case#%d] TagStore got corrupted after deletion of %q.", i, expectedResult)
				}
			}
		}
	}
}

func TestSetTag(t *testing.T) {
	tmp, err := utils.TestDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	store := mkTestTagStore(tmp, t)
	defer store.graph.driver.Cleanup()

	runSetTagCases(t, store, "")
}

func TestSetTagWithAdditionalRegistry(t *testing.T) {
	tmp, err := utils.TestDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	store := mkTestTagStore(tmp, t)
	defer store.graph.driver.Cleanup()

	registry.RegistryList = append([]string{"myrepo.io"}, registry.RegistryList...)
	defer func() {
		registry.RegistryList = registry.RegistryList[1:]
	}()

	runSetTagCases(t, store, "myrepo.io")
}
