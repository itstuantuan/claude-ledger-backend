package value

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"
)

var moneyPattern = regexp.MustCompile(`^-?\d+(?:\.\d{1,2})?$`)
var quantityPattern = regexp.MustCompile(`^\d+(?:\.\d{1,3})?$`)

type Money struct{ decimal.Decimal }
type Quantity struct{ decimal.Decimal }

func (m *Money) UnmarshalJSON(data []byte) error {
	return parseStringDecimal(data, moneyPattern, &m.Decimal, "money")
}
func (q *Quantity) UnmarshalJSON(data []byte) error {
	return parseStringDecimal(data, quantityPattern, &q.Decimal, "quantity")
}
func (m Money) MarshalJSON() ([]byte, error)    { return json.Marshal(m.Decimal.StringFixed(2)) }
func (q Quantity) MarshalJSON() ([]byte, error) { return json.Marshal(q.Decimal.String()) }

func parseStringDecimal(data []byte, pattern *regexp.Regexp, target *decimal.Decimal, name string) error {
	if len(data) == 0 || data[0] != '"' {
		return fmt.Errorf("%s must be a decimal string", name)
	}
	var raw string
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return err
	}
	if !pattern.MatchString(raw) {
		return fmt.Errorf("invalid %s format", name)
	}
	parsed, err := decimal.NewFromString(raw)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}
