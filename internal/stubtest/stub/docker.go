package main

import (
	"archive/tar"
	"bytes"
	"strings"
)

// docker stands in for docker, for --from's images only Docker has. It
// logs every call to $STUB_LOG. info and image inspect fail when
// $STUB_DIR/docker-down exists. image inspect --format {{.Id}} REF finds
// an image only when $STUB_DIR/docker-image exists, and prints it (its
// ID). save -o FILE REF writes a tarball whose manifest.json names that
// image's config ($STUB_DIR/docker-saved's instead, if it exists: the tag
// moved meanwhile). Anything else fails.
func docker(c *call) int {
	c.log()
	down := func() int {
		c.say("failed to connect to the docker API")
		return 1
	}
	switch {
	case c.arg(0) == "info":
		if c.on("docker-down") {
			return down()
		}
		return 0
	case c.arg(0) == "image" && c.arg(1) == "inspect":
		switch {
		case c.on("docker-down"):
			return down()
		case c.arg(2) != "--format" || c.arg(3) != "{{.Id}}":
			c.say("docker stub: image inspect " + c.joined())
			return 2
		case !isRegular(c.path("docker-image")):
			c.say("Error response from daemon: No such image: " + c.arg(4))
			return 1
		}
		c.print(string(c.raw("docker-image", "")))
		return 0
	case c.arg(0) == "save" && c.arg(1) == "-o":
		return c.status(save(c, c.arg(2), c.arg(3)))
	}
	c.say("docker stub: " + c.joined())
	return 2
}

// save writes a tarball of ref's image to file: only its manifest.
func save(c *call, file, ref string) error {
	id := c.textOr("docker-saved", c.textOr("docker-image", ""))
	manifest := `[{"Config":"blobs/sha256/` + strings.TrimPrefix(id, "sha256:") + `","RepoTags":["` + ref + `"],"Layers":[]}]`
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	if err := w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		return err
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return writeFile(file, b.Bytes())
}
