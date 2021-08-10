/**
 * @file: codec.go
 * @created: 2021-08-03 10:04:32
 * @author: jayden (jaydenzhao@outlook.com)
 * @description: code for project
 * @last modified: 2021-08-03 10:04:34
 * @modified by: jayden (jaydenzhao@outlook.com>)
 */
package codec

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTagName = "codec"

	codecMethodTag = "convert_method="
	inseparableTag = "inseparable"
	squashTag      = "squash"
	omitemptyTag   = "omitempty"
	remainTag      = "remain"

	timeFormatTag      = "time_format="
	timeFormatUnix     = "unix"
	timeFormatUnixnano = "unixnano"
)

type IEncode interface {
	MarshalToExport() (interface{}, error)
}

type IDecode interface {
	UnmarshalFromImport(data interface{}) error
}

type DecodeHookFunc interface{}

type DecodeHookFuncType func(reflect.Type, reflect.Type, interface{}) (interface{}, error)

type DecodeHookFuncKind func(reflect.Kind, reflect.Kind, interface{}) (interface{}, error)

type DecodeHookFuncValue func(from reflect.Value, to reflect.Value) (interface{}, error)

type DecoderConfig struct {
	// 解码预处理钩子
	DecodeHook DecodeHookFunc

	//在解码过程中未使用的原始map中的键会报错
	ErrorUnused bool

	//如果设置为true，则在写入之前将字段归零。
	//例如，在解码的值放入之前将清空map
	//如果false，则将合并map。
	ZeroFields bool

	//如果true，则解码器将进行以下内容
	//“弱”转换:
	//
	//   - bools to string (true = "1", false = "0")
	//   - numbers to string (base 10)
	//   - bools to int/uint (true = 1, false = 0)
	//   - strings to int/uint (base implied by prefix)
	//   - int to bool (true if value != 0)
	//   - string to bool (accepts: 1, t, T, TRUE, true, True, 0, f, F,
	//     FALSE, false, False. Anything else is an error)
	//   - empty array = empty map and vice versa
	//   - negative numbers to overflowed uint values (base 10)
	//   - slice of maps to a merged map
	//   - single values are converted to slices if required. Each
	//     element is weakly decoded. For example: "4" can become []int{4}
	//     if the target type is an int slice.
	//
	WeaklyTypedInput bool

	//压缩嵌入式结构。
	//使用标记添加到单个结构字段。例如：
	//
	//  type Parent struct {
	//      Child `codec:",squash"`
	//  }
	Squash bool

	//元数据是包含额外元数据的结构
	//如果这是nil，则不会跟踪元数据。
	Metadata *Metadata

	//指向包含解码的结构的指针
	Result interface{}

	// 编解码器字段标签名称。
	// 默认 "codec"
	TagName string
}

type Decoder struct {
	config *DecoderConfig
}

type Metadata struct {
	// 成功解码的键
	Keys []string

	//结果中没有匹配字段的键
	Unused []string
}

func Decode(input interface{}, output interface{}) error {
	config := &DecoderConfig{
		Metadata: nil,
		Result:   output,
	}

	decoder, err := NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func WeakDecode(input, output interface{}) error {
	config := &DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
	}

	decoder, err := NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func DecodeMetadata(input interface{}, output interface{}, metadata *Metadata) error {
	config := &DecoderConfig{
		Metadata: metadata,
		Result:   output,
	}

	decoder, err := NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func WeakDecodeMetadata(input interface{}, output interface{}, metadata *Metadata) error {
	config := &DecoderConfig{
		Metadata:         metadata,
		Result:           output,
		WeaklyTypedInput: true,
	}

	decoder, err := NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func NewDecoder(config *DecoderConfig) (*Decoder, error) {
	val := reflect.ValueOf(config.Result)
	if val.Kind() != reflect.Ptr {
		return nil, errors.New("result must be a pointer")
	}

	val = val.Elem()
	if !val.CanAddr() {
		return nil, errors.New("result must be addressable (a pointer)")
	}

	if config.Metadata != nil {
		if config.Metadata.Keys == nil {
			config.Metadata.Keys = make([]string, 0)
		}

		if config.Metadata.Unused == nil {
			config.Metadata.Unused = make([]string, 0)
		}
	}

	if config.TagName == "" {
		config.TagName = defaultTagName
	}

	result := &Decoder{
		config: config,
	}

	return result, nil
}

func (d *Decoder) Decode(input interface{}) error {
	return d.decode("", input, reflect.ValueOf(d.config.Result).Elem())
}

func (d *Decoder) decode(name string, input interface{}, outVal reflect.Value) error {
	var inputVal reflect.Value
	if input != nil {
		inputVal = reflect.ValueOf(input)

		if inputVal.Kind() == reflect.Ptr && inputVal.IsNil() {
			input = nil
		}
	}

	if input == nil {
		if d.config.ZeroFields {
			outVal.Set(reflect.Zero(outVal.Type()))

			if d.config.Metadata != nil && name != "" {
				d.config.Metadata.Keys = append(d.config.Metadata.Keys, name)
			}
		}
		return nil
	}

	if !inputVal.IsValid() {
		outVal.Set(reflect.Zero(outVal.Type()))
		if d.config.Metadata != nil && name != "" {
			d.config.Metadata.Keys = append(d.config.Metadata.Keys, name)
		}
		return nil
	}

	if codec, ok := input.(IEncode); ok {
		rst, err := codec.MarshalToExport()
		if err != nil {
			return err
		}
		input = rst
	} else if d.config.DecodeHook != nil {
		var err error
		input, err = DecodeHookExec(d.config.DecodeHook, inputVal, outVal)
		if err != nil {
			return fmt.Errorf("error decoding '%s': %s", name, err)
		}
	}

	if outVal.CanInterface() {
		if codec, ok := outVal.Interface().(IDecode); ok {
			if err := codec.UnmarshalFromImport(input); err != nil {
				return err
			}
			return nil
		} else {
			outPtr := reflect.New(outVal.Type())
			if outPtr.CanSet() {
				outPtr.Set(outVal)
				if outPtr.CanInterface() {
					if codec, ok := outPtr.Interface().(IDecode); ok {
						if err := codec.UnmarshalFromImport(input); err != nil {
							return err
						}
						return nil
					}
				}
			}
		}
	}

	var err error
	outputKind := getKind(outVal)
	addMetaKey := true
	switch outputKind {
	case reflect.Bool:
		err = d.decodeBool(name, input, outVal)
	case reflect.Interface:
		err = d.decodeBasic(name, input, outVal)
	case reflect.String:
		err = d.decodeString(name, input, outVal)
	case reflect.Int:
		err = d.decodeInt(name, input, outVal)
	case reflect.Uint:
		err = d.decodeUint(name, input, outVal)
	case reflect.Float32:
		err = d.decodeFloat(name, input, outVal)
	case reflect.Struct:
		err = d.decodeStruct(name, input, outVal)
	case reflect.Map:
		err = d.decodeMap(name, input, outVal)
	case reflect.Ptr:
		addMetaKey, err = d.decodePtr(name, input, outVal)
	case reflect.Slice:
		err = d.decodeSlice(name, input, outVal)
	case reflect.Array:
		err = d.decodeArray(name, input, outVal)
	case reflect.Func:
		err = d.decodeFunc(name, input, outVal)
	default:
		return fmt.Errorf("%s: unsupported type: %s", name, outputKind)
	}

	if addMetaKey && d.config.Metadata != nil && name != "" {
		d.config.Metadata.Keys = append(d.config.Metadata.Keys, name)
	}

	return err
}

func (d *Decoder) decodeBasic(name string, data interface{}, val reflect.Value) error {
	if val.IsValid() && val.Elem().IsValid() {
		elem := val.Elem()

		copied := false
		if !elem.CanAddr() {
			copied = true

			copy := reflect.New(elem.Type())

			copy.Elem().Set(elem)

			elem = copy
		}

		if err := d.decode(name, data, elem); err != nil || !copied {
			return err
		}

		val.Set(elem.Elem())
		return nil
	}

	dataVal := reflect.ValueOf(data)

	if dataVal.Kind() == reflect.Ptr && dataVal.Type().Elem() == val.Type() {
		dataVal = reflect.Indirect(dataVal)
	}

	if !dataVal.IsValid() {
		dataVal = reflect.Zero(val.Type())
	}

	dataValType := dataVal.Type()
	if !dataValType.AssignableTo(val.Type()) {
		return fmt.Errorf(
			"'%s' expected type '%s', got '%s'",
			name, val.Type(), dataValType)
	}

	val.Set(dataVal)
	return nil
}

func (d *Decoder) decodeString(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataKind := getKind(dataVal)

	converted := true
	switch {
	case dataKind == reflect.String:
		val.SetString(dataVal.String())
	case dataKind == reflect.Bool && d.config.WeaklyTypedInput:
		if dataVal.Bool() {
			val.SetString("1")
		} else {
			val.SetString("0")
		}
	case dataKind == reflect.Int && d.config.WeaklyTypedInput:
		val.SetString(strconv.FormatInt(dataVal.Int(), 10))
	case dataKind == reflect.Uint && d.config.WeaklyTypedInput:
		val.SetString(strconv.FormatUint(dataVal.Uint(), 10))
	case dataKind == reflect.Float32 && d.config.WeaklyTypedInput:
		val.SetString(strconv.FormatFloat(dataVal.Float(), 'f', -1, 64))
	case dataKind == reflect.Slice && d.config.WeaklyTypedInput,
		dataKind == reflect.Array && d.config.WeaklyTypedInput:
		dataType := dataVal.Type()
		elemKind := dataType.Elem().Kind()
		switch elemKind {
		case reflect.Uint8:
			var uints []uint8
			if dataKind == reflect.Array {
				uints = make([]uint8, dataVal.Len(), dataVal.Len())
				for i := range uints {
					uints[i] = dataVal.Index(i).Interface().(uint8)
				}
			} else {
				uints = dataVal.Interface().([]uint8)
			}
			val.SetString(string(uints))
		default:
			converted = false
		}
	default:
		converted = false
	}

	if !converted {
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}

	return nil
}

func (d *Decoder) decodeInt(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataKind := getKind(dataVal)
	dataType := dataVal.Type()

	switch {
	case dataKind == reflect.Int:
		val.SetInt(dataVal.Int())
	case dataKind == reflect.Uint:
		val.SetInt(int64(dataVal.Uint()))
	case dataKind == reflect.Float32:
		val.SetInt(int64(dataVal.Float()))
	case dataKind == reflect.Bool && d.config.WeaklyTypedInput:
		if dataVal.Bool() {
			val.SetInt(1)
		} else {
			val.SetInt(0)
		}
	case dataKind == reflect.String && d.config.WeaklyTypedInput:
		str := dataVal.String()
		if str == "" {
			str = "0"
		}

		i, err := strconv.ParseInt(str, 0, val.Type().Bits())
		if err == nil {
			val.SetInt(i)
		} else {
			return fmt.Errorf("cannot parse '%s' as int: %s", name, err)
		}
	case dataType.PkgPath() == "encoding/json" && dataType.Name() == "Number":
		jn := data.(json.Number)
		i, err := jn.Int64()
		if err != nil {
			return fmt.Errorf(
				"error decoding json.Number into %s: %s", name, err)
		}
		val.SetInt(i)
	default:
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}

	return nil
}

func (d *Decoder) decodeUint(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataKind := getKind(dataVal)
	dataType := dataVal.Type()

	switch {
	case dataKind == reflect.Int:
		i := dataVal.Int()
		if i < 0 && !d.config.WeaklyTypedInput {
			return fmt.Errorf("cannot parse '%s', %d overflows uint",
				name, i)
		}
		val.SetUint(uint64(i))
	case dataKind == reflect.Uint:
		val.SetUint(dataVal.Uint())
	case dataKind == reflect.Float32:
		f := dataVal.Float()
		if f < 0 && !d.config.WeaklyTypedInput {
			return fmt.Errorf("cannot parse '%s', %f overflows uint",
				name, f)
		}
		val.SetUint(uint64(f))
	case dataKind == reflect.Bool && d.config.WeaklyTypedInput:
		if dataVal.Bool() {
			val.SetUint(1)
		} else {
			val.SetUint(0)
		}
	case dataKind == reflect.String && d.config.WeaklyTypedInput:
		str := dataVal.String()
		if str == "" {
			str = "0"
		}

		i, err := strconv.ParseUint(str, 0, val.Type().Bits())
		if err == nil {
			val.SetUint(i)
		} else {
			return fmt.Errorf("cannot parse '%s' as uint: %s", name, err)
		}
	case dataType.PkgPath() == "encoding/json" && dataType.Name() == "Number":
		jn := data.(json.Number)
		i, err := jn.Int64()
		if err != nil {
			return fmt.Errorf(
				"error decoding json.Number into %s: %s", name, err)
		}
		if i < 0 && !d.config.WeaklyTypedInput {
			return fmt.Errorf("cannot parse '%s', %d overflows uint",
				name, i)
		}
		val.SetUint(uint64(i))
	default:
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}

	return nil
}

func (d *Decoder) decodeBool(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataKind := getKind(dataVal)

	switch {
	case dataKind == reflect.Bool:
		val.SetBool(dataVal.Bool())
	case dataKind == reflect.Int && d.config.WeaklyTypedInput:
		val.SetBool(dataVal.Int() != 0)
	case dataKind == reflect.Uint && d.config.WeaklyTypedInput:
		val.SetBool(dataVal.Uint() != 0)
	case dataKind == reflect.Float32 && d.config.WeaklyTypedInput:
		val.SetBool(dataVal.Float() != 0)
	case dataKind == reflect.String && d.config.WeaklyTypedInput:
		b, err := strconv.ParseBool(dataVal.String())
		if err == nil {
			val.SetBool(b)
		} else if dataVal.String() == "" {
			val.SetBool(false)
		} else {
			return fmt.Errorf("cannot parse '%s' as bool: %s", name, err)
		}
	default:
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}

	return nil
}

func (d *Decoder) decodeFloat(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataKind := getKind(dataVal)
	dataType := dataVal.Type()

	switch {
	case dataKind == reflect.Int:
		val.SetFloat(float64(dataVal.Int()))
	case dataKind == reflect.Uint:
		val.SetFloat(float64(dataVal.Uint()))
	case dataKind == reflect.Float32:
		val.SetFloat(dataVal.Float())
	case dataKind == reflect.Bool && d.config.WeaklyTypedInput:
		if dataVal.Bool() {
			val.SetFloat(1)
		} else {
			val.SetFloat(0)
		}
	case dataKind == reflect.String && d.config.WeaklyTypedInput:
		str := dataVal.String()
		if str == "" {
			str = "0"
		}

		f, err := strconv.ParseFloat(str, val.Type().Bits())
		if err == nil {
			val.SetFloat(f)
		} else {
			return fmt.Errorf("cannot parse '%s' as float: %s", name, err)
		}
	case dataType.PkgPath() == "encoding/json" && dataType.Name() == "Number":
		jn := data.(json.Number)
		i, err := jn.Float64()
		if err != nil {
			return fmt.Errorf(
				"error decoding json.Number into %s: %s", name, err)
		}
		val.SetFloat(i)
	default:
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}

	return nil
}

func (d *Decoder) decodeMap(name string, data interface{}, val reflect.Value) error {
	valType := val.Type()
	valKeyType := valType.Key()
	valElemType := valType.Elem()

	valMap := val

	if valMap.IsNil() || d.config.ZeroFields {
		mapType := reflect.MapOf(valKeyType, valElemType)
		valMap = reflect.MakeMap(mapType)
	}

	dataVal := reflect.Indirect(reflect.ValueOf(data))
	switch dataVal.Kind() {
	case reflect.Map:
		return d.decodeMapFromMap(name, dataVal, val, valMap)

	case reflect.Struct:
		return d.decodeMapFromStruct(name, dataVal, val, valMap)

	case reflect.Array, reflect.Slice:
		if d.config.WeaklyTypedInput {
			return d.decodeMapFromSlice(name, dataVal, val, valMap)
		}

		fallthrough

	default:
		return fmt.Errorf("'%s' expected a map, got '%s'", name, dataVal.Kind())
	}
}

func (d *Decoder) decodeMapFromSlice(name string, dataVal reflect.Value, val reflect.Value, valMap reflect.Value) error {
	if dataVal.Len() == 0 {
		val.Set(valMap)
		return nil
	}

	for i := 0; i < dataVal.Len(); i++ {
		err := d.decode(
			name+"["+strconv.Itoa(i)+"]",
			dataVal.Index(i).Interface(), val)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) decodeMapFromMap(name string, dataVal reflect.Value, val reflect.Value, valMap reflect.Value) error {
	valType := val.Type()
	valKeyType := valType.Key()
	valElemType := valType.Elem()

	errors := make([]string, 0)

	if dataVal.Len() == 0 {
		if dataVal.IsNil() {
			if !val.IsNil() {
				val.Set(dataVal)
			}
		} else {
			val.Set(valMap)
		}

		return nil
	}

	for _, k := range dataVal.MapKeys() {
		fieldName := name + "[" + k.String() + "]"

		currentKey := reflect.Indirect(reflect.New(valKeyType))
		if err := d.decode(fieldName, k.Interface(), currentKey); err != nil {
			errors = appendErrors(errors, err)
			continue
		}

		v := dataVal.MapIndex(k).Interface()
		currentVal := reflect.Indirect(reflect.New(valElemType))
		if err := d.decode(fieldName, v, currentVal); err != nil {
			errors = appendErrors(errors, err)
			continue
		}

		valMap.SetMapIndex(currentKey, currentVal)
	}

	val.Set(valMap)

	if len(errors) > 0 {
		return &Error{errors}
	}

	return nil
}

func (d *Decoder) decodeMapFromStruct(name string, dataVal reflect.Value, val reflect.Value, valMap reflect.Value) error {
	typ := dataVal.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue
		}

		v := dataVal.Field(i)
		if !v.Type().AssignableTo(valMap.Type().Elem()) {
			return fmt.Errorf("cannot assign type '%s' to map value field of type '%s'", v.Type(), valMap.Type().Elem())
		}

		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
		}

		tagValue := f.Tag.Get(d.config.TagName)
		keyName := f.Name

		inseparable := false
		fmtMethodName := ""
		timeFormat := ""

		squash := d.config.Squash && (v.Kind() == reflect.Struct) && f.Anonymous

		if index := strings.Index(tagValue, ","); index != -1 {
			if tagValue[:index] == "-" {
				continue
			}
			if strings.Index(tagValue[index+1:], omitemptyTag) != -1 && isEmptyValue(v) {
				continue
			}

			fIndex := strings.Index(tagValue[index+1:], codecMethodTag)
			if fIndex != -1 {
				fNamePart := tagValue[index+1+fIndex:]
				parts := strings.Split(fNamePart, ",")
				if len(parts) > 0 {
					mName := strings.TrimSpace(parts[0])
					if len(mName) > 0 {
						fmtMethodName = mName[len(codecMethodTag):]
					}
				}
			}

			tIndex := strings.Index(tagValue[index+1:], timeFormatTag)
			if tIndex != -1 {
				tNamePart := tagValue[index+1+tIndex:]
				parts := strings.Split(tNamePart, ",")
				if len(parts) > 0 {
					mName := strings.TrimSpace(parts[0])
					if len(mName) > 0 {
						timeFormat = mName[len(timeFormatTag):]
					}
				}
			}

			inseparable = strings.Index(tagValue[index+1:], inseparableTag) != -1

			squash = !squash && strings.Index(tagValue[index+1:], squashTag) != -1
			if squash {
				if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
					v = v.Elem()
				}

				if v.Kind() != reflect.Struct {
					return fmt.Errorf("cannot squash non-struct type '%s'", v.Type())
				}
			}
			keyName = tagValue[:index]
		} else if len(tagValue) > 0 {
			if tagValue == "-" {
				continue
			}
			keyName = tagValue
		}

		if v.Kind() == reflect.Struct && (!inseparable && len(timeFormat) == 0) {
			x := reflect.New(v.Type())
			x.Elem().Set(v)

			vType := valMap.Type()
			vKeyType := vType.Key()
			vElemType := vType.Elem()
			mType := reflect.MapOf(vKeyType, vElemType)
			vMap := reflect.MakeMap(mType)

			addrVal := reflect.New(vMap.Type())
			reflect.Indirect(addrVal).Set(vMap)

			err := d.decode(keyName, x.Interface(), reflect.Indirect(addrVal))
			if err != nil {
				return err
			}

			vMap = reflect.Indirect(addrVal)

			if squash {
				for _, k := range vMap.MapKeys() {
					valMap.SetMapIndex(k, vMap.MapIndex(k))
				}
			} else {
				valMap.SetMapIndex(reflect.ValueOf(keyName), vMap)
			}
		} else {
			if v.CanInterface() {
				if codec, ok := v.Interface().(IEncode); ok {
					rst, err := codec.MarshalToExport()
					if err != nil {
						return err
					}
					v = reflect.ValueOf(rst)
				}
			}
			if len(fmtMethodName) > 0 {
				target := dataVal
				method := target.MethodByName(fmtMethodName)
				if !method.IsValid() {
					target = reflect.New(typ)
					reflect.Indirect(target).Set(dataVal)
					method = target.MethodByName(fmtMethodName)
				}
				if method.IsValid() {
					results := method.Call([]reflect.Value{reflect.ValueOf(f.Name), v})
					if len(results) > 0 {
						if !isEmptyValue(results[1]) {
							return results[1].Interface().(error)
						}
						v = results[0]
					}
				}
			} else {
				if len(timeFormat) > 0 {
					switch v.Type().Kind() {
					case reflect.Int64:
						switch timeFormat {
						case timeFormatUnix, timeFormatUnixnano:
						default:
							t := time.Unix(v.Interface().(int64), 0)
							tStr := t.Format(timeFormat)
							if len(tStr) > 0 {
								v = reflect.ValueOf(tStr)
							} else {
								addrVal := reflect.New(reflect.TypeOf(""))
								reflect.Indirect(addrVal).Set(reflect.ValueOf(""))
								err := d.decode(keyName, v.Interface(), reflect.Indirect(addrVal))
								if err != nil {
									return err
								}
								v = reflect.Indirect(addrVal)
							}

						}
					default:
						t, ok := v.Interface().(time.Time)
						tType := reflect.TypeOf(time.Time{})
						if ok || v.Type().ConvertibleTo(tType) {
							if !ok {
								t = v.Convert(tType).Interface().(time.Time)
							}
							switch timeFormat {
							case timeFormatUnix:
								v = reflect.ValueOf(t.Unix())
							case timeFormatUnixnano:
								v = reflect.ValueOf(t.UnixNano())
							default:
								tStr := t.Format(timeFormat)
								if len(tStr) > 0 {
									v = reflect.ValueOf(tStr)
								} else {
									addrVal := reflect.New(reflect.TypeOf(""))
									reflect.Indirect(addrVal).Set(reflect.ValueOf(""))
									err := d.decode(keyName, v.Interface(), reflect.Indirect(addrVal))
									if err != nil {
										return err
									}
									v = reflect.Indirect(addrVal)
								}

							}
						}

					}

				}
			}
			valMap.SetMapIndex(reflect.ValueOf(keyName), v)
		}
	}

	if val.CanAddr() {
		val.Set(valMap)
	}

	return nil
}

func (d *Decoder) decodePtr(name string, data interface{}, val reflect.Value) (bool, error) {
	isNil := data == nil
	if !isNil {
		switch v := reflect.Indirect(reflect.ValueOf(data)); v.Kind() {
		case reflect.Chan,
			reflect.Func,
			reflect.Interface,
			reflect.Map,
			reflect.Ptr,
			reflect.Slice:
			isNil = v.IsNil()
		}
	}
	if isNil {
		if !val.IsNil() && val.CanSet() {
			nilValue := reflect.New(val.Type()).Elem()
			val.Set(nilValue)
		}

		return true, nil
	}

	valType := val.Type()
	valElemType := valType.Elem()
	if val.CanSet() {
		realVal := val
		if realVal.IsNil() || d.config.ZeroFields {
			realVal = reflect.New(valElemType)
		}

		if err := d.decode(name, data, reflect.Indirect(realVal)); err != nil {
			return false, err
		}

		val.Set(realVal)
	} else {
		if err := d.decode(name, data, reflect.Indirect(val)); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (d *Decoder) decodeFunc(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	if val.Type() != dataVal.Type() {
		return fmt.Errorf(
			"'%s' expected type '%s', got unconvertible type '%s', value: '%v'",
			name, val.Type(), dataVal.Type(), data)
	}
	val.Set(dataVal)
	return nil
}

func (d *Decoder) decodeSlice(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataValKind := dataVal.Kind()
	valType := val.Type()
	valElemType := valType.Elem()
	sliceType := reflect.SliceOf(valElemType)

	if dataValKind != reflect.Array && dataValKind != reflect.Slice {
		if d.config.WeaklyTypedInput {
			switch {
			case dataValKind == reflect.Slice, dataValKind == reflect.Array:
				break

			case dataValKind == reflect.Map:
				if dataVal.Len() == 0 {
					val.Set(reflect.MakeSlice(sliceType, 0, 0))
					return nil
				}
				return d.decodeSlice(name, []interface{}{data}, val)

			case dataValKind == reflect.String && valElemType.Kind() == reflect.Uint8:
				return d.decodeSlice(name, []byte(dataVal.String()), val)

			default:
				return d.decodeSlice(name, []interface{}{data}, val)
			}
		}

		return fmt.Errorf(
			"'%s': source data must be an array or slice, got %s", name, dataValKind)
	}

	if dataVal.IsNil() {
		return nil
	}

	valSlice := val
	if valSlice.IsNil() || d.config.ZeroFields {
		valSlice = reflect.MakeSlice(sliceType, dataVal.Len(), dataVal.Len())
	}

	errors := make([]string, 0)

	for i := 0; i < dataVal.Len(); i++ {
		currentData := dataVal.Index(i).Interface()
		for valSlice.Len() <= i {
			valSlice = reflect.Append(valSlice, reflect.Zero(valElemType))
		}
		currentField := valSlice.Index(i)

		fieldName := name + "[" + strconv.Itoa(i) + "]"
		if err := d.decode(fieldName, currentData, currentField); err != nil {
			errors = appendErrors(errors, err)
		}
	}

	val.Set(valSlice)

	if len(errors) > 0 {
		return &Error{errors}
	}

	return nil
}

func (d *Decoder) decodeArray(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))
	dataValKind := dataVal.Kind()
	valType := val.Type()
	valElemType := valType.Elem()
	arrayType := reflect.ArrayOf(valType.Len(), valElemType)

	valArray := val

	if valArray.Interface() == reflect.Zero(valArray.Type()).Interface() || d.config.ZeroFields {
		if dataValKind != reflect.Array && dataValKind != reflect.Slice {
			if d.config.WeaklyTypedInput {
				switch {
				case dataValKind == reflect.Map:
					if dataVal.Len() == 0 {
						val.Set(reflect.Zero(arrayType))
						return nil
					}

				default:
					return d.decodeArray(name, []interface{}{data}, val)
				}
			}

			return fmt.Errorf(
				"'%s': source data must be an array or slice, got %s", name, dataValKind)

		}
		if dataVal.Len() > arrayType.Len() {
			return fmt.Errorf(
				"'%s': expected source data to have length less or equal to %d, got %d", name, arrayType.Len(), dataVal.Len())

		}

		valArray = reflect.New(arrayType).Elem()
	}

	errors := make([]string, 0)

	for i := 0; i < dataVal.Len(); i++ {
		currentData := dataVal.Index(i).Interface()
		currentField := valArray.Index(i)

		fieldName := name + "[" + strconv.Itoa(i) + "]"
		if err := d.decode(fieldName, currentData, currentField); err != nil {
			errors = appendErrors(errors, err)
		}
	}

	val.Set(valArray)

	if len(errors) > 0 {
		return &Error{errors}
	}

	return nil
}

func (d *Decoder) decodeStruct(name string, data interface{}, val reflect.Value) error {
	dataVal := reflect.Indirect(reflect.ValueOf(data))

	if dataVal.Type() == val.Type() {
		val.Set(dataVal)
		return nil
	}

	dataValKind := dataVal.Kind()
	switch dataValKind {
	case reflect.Map:
		return d.decodeStructFromMap(name, dataVal, val)

	case reflect.Struct:
		mapType := reflect.TypeOf((map[string]interface{})(nil))
		mval := reflect.MakeMap(mapType)

		addrVal := reflect.New(mval.Type())

		reflect.Indirect(addrVal).Set(mval)
		if err := d.decodeMapFromStruct(name, dataVal, reflect.Indirect(addrVal), mval); err != nil {
			return err
		}

		result := d.decodeStructFromMap(name, reflect.Indirect(addrVal), val)
		return result

	default:
		return fmt.Errorf("'%s' expected a map, got '%s'", name, dataVal.Kind())
	}
}

func (d *Decoder) decodeStructFromMap(name string, dataVal, val reflect.Value) error {
	dataValType := dataVal.Type()
	if kind := dataValType.Key().Kind(); kind != reflect.String && kind != reflect.Interface {
		return fmt.Errorf(
			"'%s' needs a map with string keys, has '%s' keys",
			name, dataValType.Key().Kind())
	}

	dataValKeys := make(map[reflect.Value]struct{})
	dataValKeysUnused := make(map[interface{}]struct{})
	for _, dataValKey := range dataVal.MapKeys() {
		dataValKeys[dataValKey] = struct{}{}
		dataValKeysUnused[dataValKey.Interface()] = struct{}{}
	}

	errors := make([]string, 0)

	structs := make([]reflect.Value, 1, 5)
	structs[0] = val

	type field struct {
		codecMethod reflect.Value
		timeFormat  string
		inseparable bool
		field       reflect.StructField
		val         reflect.Value
	}

	var remainField *field

	fields := []field{}
	for len(structs) > 0 {
		structVal := structs[0]
		structs = structs[1:]

		structType := structVal.Type()

		for i := 0; i < structType.NumField(); i++ {
			fieldType := structType.Field(i)
			fieldVal := structVal.Field(i)
			if fieldVal.Kind() == reflect.Ptr && fieldVal.Elem().Kind() == reflect.Struct {
				fieldVal = fieldVal.Elem()
			}

			squash := d.config.Squash && fieldVal.Kind() == reflect.Struct && fieldType.Anonymous
			remain := false

			fmtMethodName := ""
			timeFormat := ""
			inseparable := false

			tagParts := strings.Split(fieldType.Tag.Get(d.config.TagName), ",")
			for _, tag := range tagParts[1:] {
				if tag == squashTag {
					squash = true
					break
				}

				if tag == remainTag {
					remain = true
					break
				}

				if tag == inseparableTag {
					inseparable = true
					continue
				}
				fIndex := strings.Index(tag, codecMethodTag)
				if fIndex != -1 {
					fmtMethodName = strings.TrimSpace(tag[fIndex+len(codecMethodTag):])
					continue
				}

				tIndex := strings.Index(tag, timeFormatTag)
				if tIndex != -1 {
					timeFormat = strings.TrimSpace(tag[tIndex+len(timeFormatTag):])
				}
			}

			if squash {
				if fieldVal.Kind() != reflect.Struct {
					errors = appendErrors(errors,
						fmt.Errorf("%s: unsupported type for squash: %s", fieldType.Name, fieldVal.Kind()))
				} else {
					structs = append(structs, fieldVal)
				}
				continue
			}

			if remain {
				remainField = &field{field: fieldType, val: structVal.Field(i)}
			} else {
				fieldItem := field{field: fieldType, val: structVal.Field(i)}
				if len(fmtMethodName) > 0 {

					target := structVal
					method := target.MethodByName(fmtMethodName)
					if !method.IsValid() {
						target = reflect.New(structType)
						reflect.Indirect(target).Set(structVal)
						method = target.MethodByName(fmtMethodName)
					}
					if method.IsValid() {
						fieldItem.codecMethod = method
					}
				}
				if len(timeFormat) > 0 {
					fieldItem.timeFormat = timeFormat
				}
				if inseparable {
					fieldItem.inseparable = inseparable
				}
				fields = append(fields, fieldItem)
			}
		}
	}

	for _, f := range fields {
		field, fieldValue, timeFormat, inseparable, codecMethod := f.field, f.val, f.timeFormat, f.inseparable, f.codecMethod
		fieldName := field.Name

		tagValue := field.Tag.Get(d.config.TagName)
		tagValue = strings.SplitN(tagValue, ",", 2)[0]
		if tagValue != "" {
			fieldName = tagValue
		}

		rawMapKey := reflect.ValueOf(fieldName)
		rawMapVal := dataVal.MapIndex(rawMapKey)
		if !rawMapVal.IsValid() {
			for dataValKey := range dataValKeys {
				mK, ok := dataValKey.Interface().(string)
				if !ok {
					continue
				}

				if strings.EqualFold(mK, fieldName) {
					rawMapKey = dataValKey
					rawMapVal = dataVal.MapIndex(dataValKey)
					break
				}
			}

			if !rawMapVal.IsValid() {
				continue
			}
		}

		if !fieldValue.IsValid() {
			panic("field is not valid")
		}

		if !fieldValue.CanSet() {
			continue
		}

		delete(dataValKeysUnused, rawMapKey.Interface())

		if name != "" {
			fieldName = name + "." + fieldName
		}

		if codecMethod.IsValid() {
			results := codecMethod.Call([]reflect.Value{reflect.ValueOf(field.Name), rawMapVal})
			if len(results) > 0 {
				if !isEmptyValue(results[1]) {
					return results[1].Interface().(error)
				}
				rawMapVal = results[0]
			}
		}
		if !inseparable && len(timeFormat) == 0 {
			if err := d.decode(fieldName, rawMapVal.Interface(), fieldValue); err != nil {
				errors = appendErrors(errors, err)
			}
		} else {
			if len(timeFormat) > 0 {
				var unix int64
				var unixnano int64
				var interval int64
				switch timeFormat {
				case timeFormatUnix, timeFormatUnixnano:
					if tStr, ok := rawMapVal.Interface().(string); ok {
						tv, err := strconv.ParseInt(tStr, 10, 64)
						if err != nil {
							return err
						}
						d := time.Duration(1)
						if timeFormat == timeFormatUnixnano {
							d = time.Second
						}
						unix, unixnano = tv/int64(d), tv%int64(d)
						interval = tv
					}
				default:
					if tStr, ok := rawMapVal.Interface().(string); ok {
						t, err := time.Parse(timeFormat, tStr)
						if err != nil {
							return err
						}
						tv := t.UnixNano()
						d := time.Second
						unix, unixnano = tv/int64(d), tv%int64(d)
						interval = tv
					}
				}
				fType := fieldValue.Type()
				tType := reflect.TypeOf(time.Time{})
				if fType == tType || tType.ConvertibleTo(fType) {
					nT := time.Unix(unix, unixnano).UTC()
					fV := reflect.ValueOf(nT).Convert(fieldValue.Type())
					fieldValue.Set(fV)
				} else {
					switch fieldValue.Type().Kind() {
					case reflect.Int64:
						fieldValue.Set(reflect.ValueOf(interval))
					}
				}
			}
		}
	}

	if remainField != nil && len(dataValKeysUnused) > 0 {
		remain := map[interface{}]interface{}{}
		for key := range dataValKeysUnused {
			remain[key] = dataVal.MapIndex(reflect.ValueOf(key)).Interface()
		}

		if err := d.decodeMap(name, remain, remainField.val); err != nil {
			errors = appendErrors(errors, err)
		}

		dataValKeysUnused = nil
	}

	if d.config.ErrorUnused && len(dataValKeysUnused) > 0 {
		keys := make([]string, 0, len(dataValKeysUnused))
		for rawKey := range dataValKeysUnused {
			keys = append(keys, rawKey.(string))
		}
		sort.Strings(keys)

		err := fmt.Errorf("'%s' has invalid keys: %s", name, strings.Join(keys, ", "))
		errors = appendErrors(errors, err)
	}

	if len(errors) > 0 {
		return &Error{errors}
	}

	if d.config.Metadata != nil {
		for rawKey := range dataValKeysUnused {
			key := rawKey.(string)
			if name != "" {
				key = name + "." + key
			}

			d.config.Metadata.Unused = append(d.config.Metadata.Unused, key)
		}
	}

	return nil
}

func isEmptyValue(v reflect.Value) bool {
	switch getKind(v) {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
}

func getKind(val reflect.Value) reflect.Kind {
	kind := val.Kind()

	switch {
	case kind >= reflect.Int && kind <= reflect.Int64:
		return reflect.Int
	case kind >= reflect.Uint && kind <= reflect.Uint64:
		return reflect.Uint
	case kind >= reflect.Float32 && kind <= reflect.Float64:
		return reflect.Float32
	default:
		return kind
	}
}
