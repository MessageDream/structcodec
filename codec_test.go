/**
 * @file: codec_test.go
 * @created: 2021-08-03 03:35:41
 * @author: jayden (jaydenzhao@outlook.com)
 * @description: code for project
 * @last modified: 2021-08-03 03:35:44
 * @modified by: jayden (jaydenzhao@outlook.com>)
 */
package codec

import (
	"testing"
	"time"
)

type Source uint8

func (s Source) String() string {
	switch s {
	case 1:
		return "本地"
	default:
		return "远程"
	}
}

type SourceDisplay string

func (s SourceDisplay) Raw() Source {
	switch s {
	case "本地":
		return 1
	default:
		return 2
	}
}

type StampTime time.Time

type TestT struct {
	M2         string    `codec:"设备唯一标识"`
	Source     Source    `codec:"来源,convert_method=SourceConvert"`
	CreateTime StampTime `codec:"创建时间,time_format=2006-01-02 15:04:05"`
}

func (t TestT) SourceConvert(fieldName string, val interface{}) (interface{}, error) {
	switch s := val.(type) {
	case Source:
		return s.String(), nil
	case string:
		return SourceDisplay(s).Raw(), nil
	}
	return val, nil
}

func TestTimeCodec(t *testing.T) {
	const layout = "2006-01-02 15:04:05"
	tm, _ := time.Parse(layout, "2021-08-04 15:42:51")
	structV := TestT{
		M2:         "xxxxxxx",
		Source:     1,
		CreateTime: StampTime(tm),
	}

	var result map[string]interface{}

	codec, err := NewDecoder(&DecoderConfig{
		DecodeHook: TimeToStringHookFunc(layout),
		Result:     &result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := codec.Decode(&structV); err != nil {
		t.Fatal(err)
	}
	nStructV := TestT{}
	if err := Decode(&result, &nStructV); err != nil {
		t.Fatal(err)
	}
	nStr := time.Time(nStructV.CreateTime).Format(layout)
	tStr := result["创建时间"]
	if tStr != nStr {
		t.Fatalf("should be %s,but %s", tStr, nStr)
	}
}

type MarshalTime time.Time

func (t MarshalTime) MarshalToExport() (interface{}, error) {
	return time.Time(t).Format("2006-01-02 15:04:05"), nil
}

func (t *MarshalTime) UnmarshalFromImport(data interface{}) error {
	tm, err := time.Parse("2006-01-02 15:04:05", data.(string))
	if err != nil {
		return err
	}
	*t = MarshalTime(tm)
	return nil
}

type TestM struct {
	M2         string      `codec:"设备唯一标识"`
	Source     Source      `codec:"来源,convert_method=SourceConvert"`
	CreateTime MarshalTime `codec:"创建时间,inseparable"`
}

func (t TestM) SourceConvert(fieldName string, val interface{}) (interface{}, error) {
	switch s := val.(type) {
	case Source:
		return s.String(), nil
	case string:
		return SourceDisplay(s).Raw(), nil
	}
	return val, nil
}

func TestMarshalCodec(t *testing.T) {
	const layout = "2006-01-02 15:04:05"
	tm, _ := time.Parse(layout, "2021-08-04 15:42:51")
	structV := TestM{
		M2:         "xxxxxxx",
		Source:     1,
		CreateTime: MarshalTime(tm),
	}

	var result map[string]interface{}

	codec, err := NewDecoder(&DecoderConfig{
		DecodeHook: TimeToStringHookFunc("2006-01-02 15:04:05"),
		Result:     &result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := codec.Decode(&structV); err != nil {
		t.Fatal(err)
	}

	nStructV := TestT{}
	if err := Decode(&result, &nStructV); err != nil {
		t.Fatal(err)
	}
	nStr := time.Time(nStructV.CreateTime).Format(layout)
	tStr := result["创建时间"]
	if tStr != nStr {
		t.Fatalf("should be %s,but %s", tStr, nStr)
	}
}
