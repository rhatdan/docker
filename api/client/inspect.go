package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/docker/registry"
)

// CmdInspect displays low-level information on one or more containers or images.
//
// Usage: docker inspect [OPTIONS] CONTAINER|IMAGE [CONTAINER|IMAGE...]
func (cli *DockerCli) CmdInspect(args ...string) error {
	cmd := cli.Subcmd("inspect", []string{"CONTAINER|IMAGE [CONTAINER|IMAGE...]"}, "Return low-level information on a container or image", true)
	tmplStr := cmd.String([]string{"f", "#format", "-format"}, "", "Format the output using the given go template")
	remote := cmd.Bool([]string{"r", "-remote"}, false, "Inspect remote images")
	cmd.Require(flag.Min, 1)

	cmd.ParseFlags(args, true)

	var tmpl *template.Template
	if *tmplStr != "" {
		var err error
		if tmpl, err = template.New("").Funcs(funcMap).Parse(*tmplStr); err != nil {
			return StatusError{StatusCode: 64,
				Status: "Template parsing error: " + err.Error()}
		}
	}

	indented := new(bytes.Buffer)
	indented.WriteString("[\n")
	status := 0
	isImage := false

	for _, name := range cmd.Args() {
		var (
			err        error
			stream     io.ReadCloser
			hdr        http.Header
			statusCode int
		)
		if !*remote {
			stream, hdr, statusCode, err = cli.call("GET", "/containers/"+name+"/json", nil, nil)
		}
		if *remote || err != nil {
			if *remote {
				taglessRemote, _ := parsers.ParseRepositoryTag(name)
				// Resolve the Repository name from fqn to RepositoryInfo
				repoInfo, err := registry.ParseRepositoryInfo(taglessRemote)
				if err != nil {
					return err
				}
				v := url.Values{}
				v.Set("remote", "1")
				stream, statusCode, err = cli.clientRequestAttemptLogin("GET", "/images/"+name+"/json?"+v.Encode(), nil, nil, repoInfo.Index, "inspect")
			} else {
				stream, hdr, statusCode, err = cli.call("GET", "/images/"+name+"/json", nil, nil)
			}
			isImage = true
			if err != nil || statusCode != http.StatusOK {
				if (err != nil && strings.Contains(err.Error(), "No such")) || statusCode == http.StatusNotFound {
					if *remote {
						fmt.Fprintf(cli.err, "Error: No such image: %s\n", name)
					} else {
						fmt.Fprintf(cli.err, "Error: No such image or container: %s\n", name)
					}
				} else if err != nil {
					fmt.Fprintf(cli.err, "%s", err)
				} else {
					fmt.Fprintf(cli.err, "Image lookup failed with status %d (%s)\n", statusCode, http.StatusText(statusCode))
				}
				status = 1
				continue
			}
		}
		obj, _, err := readBody(stream, hdr, statusCode, err)

		if tmpl == nil {
			if *remote {
				rdr := bytes.NewReader(obj)
				dec := json.NewDecoder(rdr)

				remoteImage := types.RemoteImageInspect{}
				if err := dec.Decode(&remoteImage); err != nil {
					fmt.Fprintf(cli.err, "%s\n", err)
				} else {
					_, ref := registry.SplitReposName(name, false)
					if remoteImage.Tag != "" {
						ref += ":" + remoteImage.Tag
					}
					if remoteImage.Digest != "" {
						ref += "@" + remoteImage.Digest
					}
					logrus.Debugf("Inspecting image %s from %s registry", ref, remoteImage.Registry)
					encoded, err := json.Marshal(&remoteImage.ImageInspectBase)
					if err != nil {
						fmt.Fprintf(cli.err, "%s\n", err)
					} else {
						obj = encoded
					}
				}
			}
			if err = json.Indent(indented, obj, "", "    "); err != nil {
				fmt.Fprintf(cli.err, "%s\n", err)
				status = 1
				continue
			}
		} else {
			rdr := bytes.NewReader(obj)
			dec := json.NewDecoder(rdr)

			if isImage {
				if *remote {
					remoteImage := types.RemoteImageInspect{}
					if err := dec.Decode(&remoteImage); err != nil {
						fmt.Fprintf(cli.err, "%s\n", err)
						status = 1
						continue
					}
					_, ref := registry.SplitReposName(name, false)
					if remoteImage.Tag != "" {
						ref += ":" + remoteImage.Tag
					}
					if remoteImage.Digest != "" {
						ref += "@" + remoteImage.Digest
					}
					logrus.Debugf("Inspecting image %s from %s registry", ref, remoteImage.Registry)
					err = tmpl.Execute(cli.out, &remoteImage.ImageInspectBase)
				} else {
					inspPtr := types.ImageInspect{}
					if err := dec.Decode(&inspPtr); err != nil {
						fmt.Fprintf(cli.err, "%s\n", err)
						status = 1
						continue
					}
					err = tmpl.Execute(cli.out, inspPtr)
				}
				if err != nil {
					rdr.Seek(0, 0)
					var raw interface{}
					if err := dec.Decode(&raw); err != nil {
						return err
					}
					if err = tmpl.Execute(cli.out, raw); err != nil {
						return err
					}
				}
			} else {
				inspPtr := types.ContainerJSON{}
				if err := dec.Decode(&inspPtr); err != nil {
					fmt.Fprintf(cli.err, "%s\n", err)
					status = 1
					continue
				}
				if err := tmpl.Execute(cli.out, inspPtr); err != nil {
					rdr.Seek(0, 0)
					var raw interface{}
					if err := dec.Decode(&raw); err != nil {
						return err
					}
					if err = tmpl.Execute(cli.out, raw); err != nil {
						return err
					}
				}
			}
			cli.out.Write([]byte{'\n'})
		}
		indented.WriteString(",")
	}

	if indented.Len() > 1 {
		// Remove trailing ','
		indented.Truncate(indented.Len() - 1)
	}
	indented.WriteString("]\n")

	if tmpl == nil {
		// Note that we will always write "[]" when "-f" isn't specified,
		// to make sure the output would always be array, see
		// https://github.com/docker/docker/pull/9500#issuecomment-65846734
		if _, err := io.Copy(cli.out, indented); err != nil {
			return err
		}
	}

	if status != 0 {
		return StatusError{StatusCode: status}
	}
	return nil
}
