package scan

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var (
	structSpecMutex sync.RWMutex
	structSpecCache = make(map[reflect.Type]*structSpec)
)

type fieldSpec struct {
	name      string
	index     []int
	omitEmpty bool
}

type structSpec struct {
	m map[string]*fieldSpec
	l []*fieldSpec
}

type Error string

func cannotConvert(d reflect.Value, s interface{}) error {
	var sname string
	switch s.(type) {
	case string:
		sname = "simple string"
	case Error:
		sname = "error"
	case int64:
		sname = "integer"
	case []byte:
		sname = "bulk string"
	case []interface{}:
		sname = "array"
	case nil:
		sname = "nil"
	default:
		sname = reflect.TypeOf(s).String()
	}
	return fmt.Errorf("cannot convert from %s to %s", sname, d.Type())
}

var errScanStructValue = errors.New("scan.ScanStruct: value must be non-nil pointer to a struct")

func convertAssignNil(d reflect.Value) (err error) {
	switch d.Type().Kind() {
	case reflect.Slice, reflect.Interface:
		d.Set(reflect.Zero(d.Type()))
	default:
		err = cannotConvert(d, nil)
	}
	return err
}

func convertAssignBulkString(t reflect.StructField, d reflect.Value, s []byte) (err error) {
	switch d.Type().Kind() {
	case reflect.Slice:
		// Handle []byte destination here to avoid unnecessary
		// []byte -> string -> []byte converion.
		if d.Type().Elem().Kind() == reflect.Uint8 {
			d.SetBytes(s)
		} else {
			err = cannotConvert(d, s)
		}
	case reflect.Ptr:
		if d.CanInterface() && d.CanSet() {
			if s == nil {
				if d.IsNil() {
					return nil
				}

				d.Set(reflect.Zero(d.Type()))
				return nil
			}

			if d.IsNil() {
				d.Set(reflect.New(d.Type().Elem()))
			}

			if sc, ok := d.Interface().(Scanner); ok {
				return sc.Scan(s, t.Tag)
			}
		}
		err = convertAssignString(d, string(s))
	default:
		err = convertAssignString(d, string(s))
	}
	return err
}

func convertAssignString(d reflect.Value, s string) (err error) {
	switch d.Type().Kind() {
	case reflect.Float32, reflect.Float64:
		var x float64
		x, err = strconv.ParseFloat(s, d.Type().Bits())
		d.SetFloat(x)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var x int64
		x, err = strconv.ParseInt(s, 10, d.Type().Bits())
		d.SetInt(x)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var x uint64
		x, err = strconv.ParseUint(s, 10, d.Type().Bits())
		d.SetUint(x)
	case reflect.Bool:
		var x bool
		x, err = strconv.ParseBool(s)
		d.SetBool(x)
	case reflect.String:
		d.SetString(s)
	case reflect.Slice:
		if d.Type().Elem().Kind() == reflect.Uint8 {
			d.SetBytes([]byte(s))
		} else {
			err = cannotConvert(d, s)
		}
	case reflect.Ptr:
		err = convertAssignString(d.Elem(), s)
	default:
		err = cannotConvert(d, s)
	}
	return
}

func convertAssignValue(t reflect.StructField, d reflect.Value, s interface{}) (err error) {
	if d.Kind() != reflect.Ptr {
		if d.CanAddr() {
			d2 := d.Addr()
			if d2.CanInterface() {
				if scanner, ok := d2.Interface().(Scanner); ok {
					return scanner.Scan(s, t.Tag)
				}
			}
		}
	} else if d.CanInterface() {
		// Already a reflect.Ptr
		if d.IsNil() {
			d.Set(reflect.New(d.Type().Elem()))
		}
		if scanner, ok := d.Interface().(Scanner); ok {
			return scanner.Scan(s, t.Tag)
		}
	}

	switch s := s.(type) {
	case nil:
		err = convertAssignNil(d)
	case []byte:
		err = convertAssignBulkString(t, d, s)
	case int64:
		err = convertAssignInt(d, s)
	case string:
		err = convertAssignString(d, s)
	case Error:
		err = convertAssignError(d, s)
	default:
		err = cannotConvert(d, s)
	}
	return err
}

func convertAssignInt(d reflect.Value, s int64) (err error) {
	switch d.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		d.SetInt(s)
		if d.Int() != s {
			err = strconv.ErrRange
			d.SetInt(0)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if s < 0 {
			err = strconv.ErrRange
		} else {
			x := uint64(s)
			d.SetUint(x)
			if d.Uint() != x {
				err = strconv.ErrRange
				d.SetUint(0)
			}
		}
	case reflect.Bool:
		d.SetBool(s != 0)
	default:
		err = cannotConvert(d, s)
	}
	return
}

func convertAssignError(d reflect.Value, s Error) (err error) {
	if d.Kind() == reflect.String {
		d.SetString(string(s))
	} else if d.Kind() == reflect.Slice && d.Type().Elem().Kind() == reflect.Uint8 {
		d.SetBytes([]byte(s))
	} else {
		err = cannotConvert(d, s)
	}
	return
}

func (ss *structSpec) fieldSpec(name string) *fieldSpec {
	return ss.m[name]
}

func compileStructSpec(t reflect.Type, depth map[string]int, index []int, ss *structSpec) {
LOOP:
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		switch {
		case f.PkgPath != "" && !f.Anonymous:
			// Ignore unexported fields.
		case f.Anonymous:
			switch f.Type.Kind() {
			case reflect.Struct:
				compileStructSpec(f.Type, depth, append(index, i), ss)
			case reflect.Ptr:
				// TODO(steve): Protect against infinite recursion.
				if f.Type.Elem().Kind() == reflect.Struct {
					compileStructSpec(f.Type.Elem(), depth, append(index, i), ss)
				}
			}
		default:
			fs := &fieldSpec{name: f.Name}
			tag := f.Tag.Get("scan")

			var (
				p string
			)
			first := true

			for len(tag) > 0 {
				i := strings.IndexByte(tag, ',')
				if i < 0 {
					p, tag = tag, ""
				} else {
					p, tag = tag[:i], tag[i+1:]
				}
				if p == "-" {
					continue LOOP
				}
				if first && len(p) > 0 {
					fs.name = p
					first = false
				} else {
					switch p {
					case "omitempty":
						fs.omitEmpty = true
					default:
						panic(fmt.Errorf("scan: unknown field tag %s for type %s", p, t.Name()))
					}
				}
			}

			d, found := depth[fs.name]
			if !found {
				d = 1 << 30 //1073741824
			}
			switch {
			case len(index) == d:
				// At same depth, remove from result.
				delete(ss.m, fs.name)
				j := 0
				for i := 0; i < len(ss.l); i++ {
					if fs.name != ss.l[i].name {
						ss.l[j] = ss.l[i]
						j += 1
					}
				}
				ss.l = ss.l[:j]
			case len(index) < d:
				fs.index = make([]int, len(index)+1)
				copy(fs.index, index)
				fs.index[len(index)] = i
				depth[fs.name] = len(index)
				ss.m[fs.name] = fs
				ss.l = append(ss.l, fs)
			}
		}
	}
}

func structSpecForType(t reflect.Type) *structSpec {
	structSpecMutex.RLock()
	ss, found := structSpecCache[t]
	structSpecMutex.RUnlock()
	if found {
		return ss
	}

	structSpecMutex.Lock()
	defer structSpecMutex.Unlock()
	ss, found = structSpecCache[t]
	if found {
		return ss
	}

	ss = &structSpec{m: make(map[string]*fieldSpec)}
	compileStructSpec(t, make(map[string]int), nil, ss)
	structSpecCache[t] = ss
	return ss
}

func ScanStruct(src map[string]interface{}, dest interface{}) error {
	d := reflect.ValueOf(dest)
	if d.Kind() != reflect.Ptr || d.IsNil() {
		return errScanStructValue
	}
	d = d.Elem()
	if d.Kind() != reflect.Struct {
		return errScanStructValue
	}
	ss := structSpecForType(d.Type())

	for k, v := range src {
		if vv, ok := v.(map[string]interface{}); ok {
			rv := reflect.ValueOf(dest)
			rv = rv.Elem()
			for i := 0; i < rv.Type().NumField(); i++ {
				f := rv.Type().Field(i)
				tag := f.Tag.Get("scan")
				if tag == k {
					rvv := reflect.ValueOf(dest)
					rvv = rvv.Elem()
					ScanStruct(vv, rvv.FieldByName(f.Name).Interface())
				}
			}
		} else {
			fs := ss.fieldSpec(k)
			if fs == nil {
				continue
			}
			if err := convertAssignValue(d.Type().FieldByIndex(fs.index), d.FieldByIndex(fs.index), v); err != nil {
				return fmt.Errorf("scan.ScanStruct: cannot assign field %s: %v", fs.name, err)
			}
		}
	}
	return nil
}
