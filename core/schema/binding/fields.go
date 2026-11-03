package binding

import (
	"reflect"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/common"
	creflect "github.com/asaidimu/go-anansi/v8/core/reflect"
)

// ---------- Cached Field Metadata ----------

// bindFieldInfo is the binder's per-field record: the flattened index path
// into the target struct, the field's schema name (from the anansi tag), and
// the original StructField for error context. omitEmpty and isSystemEmbed
// are only consumed by the struct walker (walk.go); the reader ignores them.
type bindFieldInfo struct {
	index         []int
	name          string
	structField   reflect.StructField
	omitEmpty     bool
	isSystemEmbed bool
}

type bindFieldsKey struct {
	t   reflect.Type
	tag string
}

type cachedBindFields struct {
	fields []bindFieldInfo
	err    error
}

// bindFieldsCache caches flattened field metadata per (type, tag). Like the
// data implementation it replaced, parse errors are cached too so a
// malformed struct fails fast on every subsequent bind.
var bindFieldsCache sync.Map // bindFieldsKey -> *cachedBindFields

func resolveBindTagChain(customTag string) []string {
	if customTag == "" {
		return []string{AnansiTag}
	}
	return []string{customTag, AnansiTag}
}

func bindFieldsFor(t reflect.Type, tag string) ([]bindFieldInfo, error) {
	key := bindFieldsKey{t: t, tag: tag}
	if v, ok := bindFieldsCache.Load(key); ok {
		info := v.(*cachedBindFields)
		return info.fields, info.err
	}

	fields, err := buildBindFields(t, nil, resolveBindTagChain(tag), false)
	info := &cachedBindFields{fields: fields, err: err}
	bindFieldsCache.Store(key, info)
	return fields, err
}

// buildBindFields walks t's fields, flattening anonymous struct embeds and
// resolving each field's schema name through the tag chain. Tag values are
// read via the core/reflect registry (cached per type, zero-copy) rather
// than reflect.StructTag.Get. isSystemEmbed marks the whole subtree as
// system-embedded (identity fields), propagated from enclosing embeds.
func buildBindFields(t reflect.Type, indexPrefix []int, tagChain []string, isSystemEmbed bool) ([]bindFieldInfo, error) {
	var fields []bindFieldInfo

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		indexPath := append(append([]int(nil), indexPrefix...), i)

		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			// System-model embeds (e.g. DocumentModel) mark their whole
			// subtree so the struct walker can skip identity fields for
			// partial writes.
			sysEmbed := isSystemEmbed || IsSystemModelType(f.Type)
			subFields, err := buildBindFields(f.Type, indexPath, tagChain, sysEmbed)
			if err != nil {
				return nil, err
			}
			fields = append(fields, subFields...)
			continue
		}

		// Unexported fields are excluded: they cannot be set through
		// reflection, and the core/reflect tag registry (the shared source of
		// tag metadata) excludes them as well.
		if f.PkgPath != "" {
			continue
		}

		var docTag string
		for _, tagName := range tagChain {
			tg, ok := creflect.FieldTagOf(t, f.Name, tagName)
			if !ok {
				continue
			}
			v, _ := tg.Value()
			if v != "" && v != "-" {
				docTag = v
				break
			}
		}
		if docTag == "" {
			continue
		}

		fieldName, omitEmpty, sysErr := parseBindTag(docTag)
		if sysErr != nil {
			return nil, sysErr.WithPath(f.Name)
		}

		fields = append(fields, bindFieldInfo{
			index:         indexPath,
			name:          fieldName,
			structField:   f,
			omitEmpty:     omitEmpty,
			isSystemEmbed: isSystemEmbed,
		})
	}

	return fields, nil
}

// parseBindTag parses the anansi tag value grammar used for binding:
// the first comma-delimited token is the field name, bare options are flags
// (only "omitempty" is meaningful to binding), and key=value options are
// schema-only and silently ignored.
func parseBindTag(tag string) (string, bool, *common.SystemError) {
	commaIdx := strings.IndexByte(tag, ',')
	if commaIdx == -1 {
		name := strings.TrimSpace(tag)
		if name == "" {
			return "", false, ErrBindEmptyFieldName
		}
		return name, false, nil
	}

	name := strings.TrimSpace(tag[:commaIdx])
	if name == "" {
		return "", false, ErrBindEmptyFieldName
	}

	var omitempty bool
	rest := tag[commaIdx+1:]
	for len(rest) > 0 {
		var opt string
		if idx := strings.IndexByte(rest, ','); idx != -1 {
			opt = strings.TrimSpace(rest[:idx])
			rest = rest[idx+1:]
		} else {
			opt = strings.TrimSpace(rest)
			rest = ""
		}

		if opt == "omitempty" {
			omitempty = true
		} else if strings.Contains(opt, "=") {
			// schema-only options silently ignored by binding layer
		} else if opt != "" {
			return "", false, ErrBindUnknownDocTagOption.WithMessagef("unknown option: %q", opt)
		}
	}

	return name, omitempty, nil
}
