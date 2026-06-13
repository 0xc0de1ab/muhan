package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/0xc0de1ab/muhan/internal/migrate/objectmap"
	"github.com/0xc0de1ab/muhan/internal/migrate/protoresolve"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type Report struct {
	File                string                              `json:"file"`
	RootIDPrefix        string                              `json:"rootIdPrefix"`
	Location            model.ObjectLocation                `json:"location"`
	ObjectInstances     []model.ObjectInstance              `json:"objectInstances"`
	PrototypeResolution objectmap.PrototypeResolutionCounts `json:"prototypeResolution"`
	Warnings            []string                            `json:"warnings,omitempty"`
}

func main() {
	file := flag.String("file", "", "legacy bank or object-tree file")
	root := flag.String("root", "", "legacy Muhan data root; when set, resolve object prototypes from objmon")
	prefix := flag.String("prefix", "", "object instance id prefix; default is objinst:<file base>")
	room := flag.String("room", "", "root holder room id")
	creature := flag.String("creature", "", "root holder creature id")
	container := flag.String("container", "", "root holder container object instance id")
	strict := flag.Bool("strict", false, "reject trailing bytes after the object tree")
	jsonOut := flag.Bool("json", false, "write mapped object instances as JSON")
	flag.Parse()

	report, err := buildReport(*file, *root, *prefix, *room, *creature, *container, !*strict)
	if err != nil {
		fmt.Fprintf(os.Stderr, "objectmap: %v\n", err)
		os.Exit(2)
	}
	if err := writeReport(os.Stdout, report, *jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "objectmap: %v\n", err)
		os.Exit(2)
	}
}

func buildReport(file, root, prefix, room, creature, container string, allowTrailing bool) (Report, error) {
	if file == "" {
		return Report{}, fmt.Errorf("-file is required")
	}
	var prefixWarning string
	if prefix == "" {
		prefix, prefixWarning = defaultRootIDPrefix(file)
	}

	location, warnings, err := rootLocation(prefix, room, creature, container)
	if err != nil {
		return Report{}, err
	}
	if prefixWarning != "" {
		warnings = append(warnings, prefixWarning)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return Report{}, fmt.Errorf("read %s: %w", file, err)
	}

	var resolver protoresolve.ObjectPrototypeResolver
	if root != "" {
		built, err := protoresolve.BuildObjectResolver(root)
		if err != nil {
			return Report{}, fmt.Errorf("build prototype resolver: %w", err)
		}
		resolver = built
	}
	result, err := objectmap.MapObjectFileWithOptions(prefix, location, data, allowTrailing, objectmap.Options{
		PrototypeResolver: resolver,
		SourcePath:        file,
	})
	if err != nil {
		return Report{}, fmt.Errorf("map %s: %w", file, err)
	}
	warnings = append(warnings, result.Warnings...)

	return Report{
		File:                filepath.ToSlash(file),
		RootIDPrefix:        prefix,
		Location:            location,
		ObjectInstances:     result.Objects,
		PrototypeResolution: result.PrototypeResolution,
		Warnings:            warnings,
	}, nil
}

func defaultRootIDPrefix(file string) (string, string) {
	base := filepath.Base(file)
	if utf8.ValidString(base) && !strings.ContainsRune(base, utf8.RuneError) {
		return "objinst:" + base, ""
	}
	sum := sha256.Sum256([]byte(base))
	prefix := fmt.Sprintf("objinst:file:%x", sum[:6])
	return prefix, fmt.Sprintf("file base %q is not stable UTF-8; using hashed prefix %q", base, prefix)
}

func rootLocation(rootIDPrefix, room, creature, container string) (model.ObjectLocation, []string, error) {
	specified := 0
	if room != "" {
		specified++
	}
	if creature != "" {
		specified++
	}
	if container != "" {
		specified++
	}
	if specified > 1 {
		return model.ObjectLocation{}, nil, fmt.Errorf("only one of -room, -creature, or -container may be set")
	}

	switch {
	case room != "":
		return model.ObjectLocation{RoomID: model.RoomID(room)}, nil, nil
	case creature != "":
		return model.ObjectLocation{CreatureID: model.CreatureID(creature)}, nil, nil
	case container != "":
		return model.ObjectLocation{ContainerID: model.ObjectInstanceID(container)}, nil, nil
	default:
		id := model.ObjectInstanceID(rootIDPrefix + ":external")
		return model.ObjectLocation{ContainerID: id}, []string{
			fmt.Sprintf("root location not supplied; using synthetic container %q", id),
		}, nil
	}
}

func writeReport(w io.Writer, report Report, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintf(w, "file: %s\n", report.File)
	fmt.Fprintf(w, "rootIdPrefix: %s\n", report.RootIDPrefix)
	fmt.Fprintf(w, "objects: %d\n", len(report.ObjectInstances))
	fmt.Fprintf(w, "prototypeResolved: %d\n", report.PrototypeResolution.ResolvedExact)
	fmt.Fprintf(w, "prototypeSynthetic: %d\n", report.PrototypeResolution.Synthetic)
	fmt.Fprintf(w, "prototypeAmbiguous: %d\n", report.PrototypeResolution.AmbiguousSynthetic)
	fmt.Fprintf(w, "warnings: %d\n", len(report.Warnings))
	for _, warning := range report.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	for _, object := range report.ObjectInstances {
		fmt.Fprintf(w, "object: %s prototype=%s contents=%d\n", object.ID, object.PrototypeID, len(object.Contents.ObjectIDs))
	}
	return nil
}
