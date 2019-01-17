package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	pb "github.com/linkerd/linkerd2/controller/gen/public"
)

type version struct {
	channel  string
	revision string
}

// Version is updated automatically as part of the build process
//
// DO NOT EDIT
var Version = undefinedVersion

const (
	undefinedVersion = "undefined"
	versionCheckURL  = "https://versioncheck.linkerd.io/version.json?version=%s&uuid=%s&source=%s"
)

func init() {
	// Use `$LINKERD_CONTAINER_VERSION_OVERRIDE` as the version only if the
	// version wasn't set at link time to minimize the chance of using it
	// unintentionally. This mechanism allows the version to be bound at
	// container build time instead of at executable link time to improve
	// incremental rebuild efficiency.
	if Version == undefinedVersion {
		override := os.Getenv("LINKERD_CONTAINER_VERSION_OVERRIDE")
		if override != "" {
			Version = override
		}
	}
}

func (v version) String() string {
	return fmt.Sprintf("%s-%s", v.channel, v.revision)
}

// CheckClientVersion validates whether the Linkerd Public API client's version
// matches an expected version.
func CheckClientVersion(expectedVersion string) error {
	if Version != expectedVersion {
		return versionMismatchError(expectedVersion, Version)
	}

	return nil
}

// GetServerVersion returns the Linkerd Public API server version
func GetServerVersion(apiClient pb.ApiClient) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, err := apiClient.Version(ctx, &pb.Empty{})
	if err != nil {
		return "", err
	}

	return rsp.GetReleaseVersion(), nil
}

// CheckServerVersion validates whether the Linkerd Public API server's version
// matches an expected version.
func CheckServerVersion(apiClient pb.ApiClient, expectedVersion string) error {
	releaseVersion, err := GetServerVersion(apiClient)
	if err != nil {
		return err
	}

	if releaseVersion != expectedVersion {
		return versionMismatchError(expectedVersion, releaseVersion)
	}

	return nil
}

// GetLatestVersion performs an online request to check for the latest Linkerd
// version.
func GetLatestVersion(uuid string, source string) (string, error) {
	url := fmt.Sprintf(versionCheckURL, Version, uuid, source)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		return "", fmt.Errorf("Unexpected versioncheck response: %s", rsp.Status)
	}

	bytes, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", err
	}

	var versionRsp map[string]string
	err = json.Unmarshal(bytes, &versionRsp)
	if err != nil {
		return "", err
	}

	parsed, err := parseVersion(Version)
	if err != nil {
		return "", err
	}

	version, ok := versionRsp[parsed.channel]
	if !ok {
		return "", fmt.Errorf("unsupported version channel: %s", parsed.channel)
	}

	return version, nil
}

func parseVersion(v string) (version, error) {
	if parts := strings.SplitN(v, "-", 2); len(parts) == 2 {
		return version{
			channel:  parts[0],
			revision: parts[1],
		}, nil
	}
	return version{}, fmt.Errorf("unsupported version format: %s", v)
}

func versionMismatchError(expectedVersion, actualVersion string) error {
	actual, err := parseVersion(actualVersion)
	if err != nil {
		return fmt.Errorf("failed to parse actual version: %s", err)
	}
	expected, err := parseVersion(expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to parse expected version: %s", err)
	}

	if actual.channel != expected.channel {
		return fmt.Errorf("mismatched channels: running %s but retrieved %s",
			actual, expected)
	}

	return fmt.Errorf("is running version %s but the latest %s version is %s",
		actual.revision, actual.channel, expected.revision)
}
