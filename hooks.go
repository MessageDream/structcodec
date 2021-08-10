/**
 * @file: hooks.go
 * @created: 2021-08-03 10:04:20
 * @author: jayden (jaydenzhao@outlook.com)
 * @description: code for project
 * @last modified: 2021-08-03 10:04:23
 * @modified by: jayden (jaydenzhao@outlook.com>)
 */
package codec

import (
	"encoding"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func typedDecodeHook(h DecodeHookFunc) DecodeHookFunc {
	var f1 DecodeHookFuncType
	var f2 DecodeHookFuncKind
	var f3 DecodeHookFuncValue

	potential := []interface{}{f1, f2, f3}

	v := reflect.ValueOf(h)
	vt := v.Type()
	for _, raw := range potential {
		pt := reflect.ValueOf(raw).Type()
		if vt.ConvertibleTo(pt) {
			return v.Convert(pt).Interface()
		}
	}

	return nil
}

func DecodeHookExec(
	raw DecodeHookFunc,
	from reflect.Value, to reflect.Value) (interface{}, error) {

	switch f := typedDecodeHook(raw).(type) {
	case DecodeHookFuncType:
		return f(from.Type(), to.Type(), from.Interface())
	case DecodeHookFuncKind:
		return f(from.Kind(), to.Kind(), from.Interface())
	case DecodeHookFuncValue:
		return f(from, to)
	default:
		return nil, errors.New("invalid decode hook signature")
	}
}

func ComposeDecodeHookFunc(fs ...DecodeHookFunc) DecodeHookFunc {
	return func(f reflect.Value, t reflect.Value) (interface{}, error) {
		var err error
		var data interface{}
		newFrom := f
		for _, f1 := range fs {
			data, err = DecodeHookExec(f1, newFrom, t)
			if err != nil {
				return nil, err
			}
			newFrom = reflect.ValueOf(data)
		}

		return data, nil
	}
}

func StringToSliceHookFunc(sep string) DecodeHookFunc {
	return func(
		f reflect.Kind,
		t reflect.Kind,
		data interface{}) (interface{}, error) {
		if f != reflect.String || t != reflect.Slice {
			return data, nil
		}

		raw := data.(string)
		if raw == "" {
			return []string{}, nil
		}

		return strings.Split(raw, sep), nil
	}
}

func StringToTimeDurationHookFunc() DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(time.Duration(5)) {
			return data, nil
		}

		return time.ParseDuration(data.(string))
	}
}

func StringToIPHookFunc() DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(net.IP{}) {
			return data, nil
		}

		ip := net.ParseIP(data.(string))
		if ip == nil {
			return net.IP{}, fmt.Errorf("failed parsing ip %v", data)
		}

		return ip, nil
	}
}

func StringToIPNetHookFunc() DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(net.IPNet{}) {
			return data, nil
		}

		_, net, err := net.ParseCIDR(data.(string))
		return net, err
	}
}

func StringToTimeHookFunc(layout string) DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(time.Time{}) {
			return data, nil
		}

		// Convert it by parsing
		return time.Parse(layout, data.(string))
	}
}

func TimeToStringHookFunc(layout string) DecodeHookFunc {
	return func(
		f reflect.Value,
		t reflect.Value) (interface{}, error) {
		data := reflect.Indirect(f).Interface()
		if f.Type() != reflect.TypeOf(&time.Time{}) {
			return data, nil
		}
		fmt.Printf("%v", t.Kind())
		if t.Kind() != reflect.String {
			return data, nil
		}

		timeStr := data.(time.Time).Format(layout)

		reflect.Indirect(t).Set(reflect.ValueOf(timeStr))

		return data, nil
	}
}

func WeaklyTypedHook(
	f reflect.Kind,
	t reflect.Kind,
	data interface{}) (interface{}, error) {
	dataVal := reflect.ValueOf(data)
	switch t {
	case reflect.String:
		switch f {
		case reflect.Bool:
			if dataVal.Bool() {
				return "1", nil
			}
			return "0", nil
		case reflect.Float32:
			return strconv.FormatFloat(dataVal.Float(), 'f', -1, 64), nil
		case reflect.Int:
			return strconv.FormatInt(dataVal.Int(), 10), nil
		case reflect.Slice:
			dataType := dataVal.Type()
			elemKind := dataType.Elem().Kind()
			if elemKind == reflect.Uint8 {
				return string(dataVal.Interface().([]uint8)), nil
			}
		case reflect.Uint:
			return strconv.FormatUint(dataVal.Uint(), 10), nil
		}
	}

	return data, nil
}

func RecursiveStructToMapHookFunc() DecodeHookFunc {
	return func(f reflect.Value, t reflect.Value) (interface{}, error) {
		if f.Kind() != reflect.Struct {
			return f.Interface(), nil
		}

		var i interface{} = struct{}{}
		if t.Type() != reflect.TypeOf(&i).Elem() {
			return f.Interface(), nil
		}

		m := make(map[string]interface{})
		t.Set(reflect.ValueOf(m))

		return f.Interface(), nil
	}
}

func TextUnmarshallerHookFunc() DecodeHookFuncType {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		result := reflect.New(t).Interface()
		unmarshaller, ok := result.(encoding.TextUnmarshaler)
		if !ok {
			return data, nil
		}
		if err := unmarshaller.UnmarshalText([]byte(data.(string))); err != nil {
			return nil, err
		}
		return result, nil
	}
}
