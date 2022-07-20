package scan

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"time"
)

type Scanner interface {
	Scan(src interface{}, tag reflect.StructTag) error
}

type Time time.Time

func (t *Time) Scan(x interface{}, tag reflect.StructTag) error {
	bs, ok := x.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", x)
	}

	tt, err := time.Parse(tag.Get("layout"), bs)
	if err != nil {
		return err
	}
	*t = Time(tt)
	return nil
}

type IP struct{ net.IP }

func (t *IP) Scan(src interface{}, tag reflect.StructTag) error {
	if t == nil {
		return errors.New("nil pointer")
	}

	switch src := src.(type) {
	case string:
		return t.UnmarshalText([]byte(src))
	case []byte:
		return t.UnmarshalText(src)
	default:
		return fmt.Errorf("cannot convert from %T to %T", src, t)
	}
}
