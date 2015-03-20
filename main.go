// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
)

const (
	manifestFile = "manifest"
)

var flagName = flag.String("name", "", "Replace name")
var flagCaps = flag.String("capability", "", "Replace capability")

func getIsolatorStr(name, value string) string {
	return fmt.Sprintf(`
		{
		    "name": "%s",
		    "value": { %s }
		}`, name, value)
}

func patchManifest(im *schema.ImageManifest) error {

	if *flagName != "" {
		name, err := types.NewACName(*flagName)
		if err != nil {
			return err
		}
		im.Name = *name
	}

	if *flagCaps != "" {
		app := im.App
		if app == nil {
			return fmt.Errorf("no app in the manifest")
		}
		isolator := app.Isolators.GetByName(types.LinuxCapabilitiesRetainSetName)
		if isolator != nil {
			return fmt.Errorf("isolator already exists")
		}

		capsList := strings.Split(*flagCaps, ",")
		caps := fmt.Sprintf(`"set": ["%s"]`,
			strings.Join(capsList, `", "`))
		isolatorStr := getIsolatorStr(types.LinuxCapabilitiesRetainSetName,
			caps)
		isolator = &types.Isolator{}
		err := isolator.UnmarshalJSON([]byte(isolatorStr))
		if err != nil {
			return err
		}
		app.Isolators = append(app.Isolators, *isolator)
	}

	return nil
}

func aciPatch() error {
	// Read stdin
	gr, err := gzip.NewReader(os.Stdin)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Write stdout
	gw := gzip.NewWriter(os.Stdout)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Iterate
Tar:
	for {
		hdr, err := tr.Next()
		switch err {
		case io.EOF:
			break Tar
		case nil:
			if filepath.Clean(hdr.Name) == manifestFile {
				bytes, err := ioutil.ReadAll(tr)
				if err != nil {
					return err
				}

				im := &schema.ImageManifest{}
				err = im.UnmarshalJSON(bytes)
				if err != nil {
					return err
				}

				err = patchManifest(im)
				if err != nil {
					return err
				}

				new_bytes, err := im.MarshalJSON()
				if err != nil {
					return err
				}

				hdr.Size = int64(len(new_bytes))
				err = tw.WriteHeader(hdr)
				if err != nil {
					return err
				}

				_, err = tw.Write(new_bytes)
				if err != nil {
					return err
				}
			} else {
				err := tw.WriteHeader(hdr)
				if err != nil {
					return err
				}
				_, err = io.Copy(tw, tr)
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("error reading tarball: %v", err)
		}
	}

	return nil

}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) != 0 {
		fmt.Println("Usage: acipatch")
		fmt.Println("         [--name=example.com/app")
		fmt.Println("         [--capability=CAP_SYS_ADMIN,CAP_NET_ADMIN]")
		fmt.Println("         < ACI_FILE > NEW_ACI_FILE")
		return
	}

	if err := aciPatch(); err != nil {
		os.Exit(1)
	}
}
