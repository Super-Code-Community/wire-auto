package bridge

import "encoding/json"

func jsonUnmarshalLine(line string, v any) error {
	return json.Unmarshal([]byte(line), v)
}
