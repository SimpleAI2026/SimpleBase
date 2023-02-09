package database

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/beevik/etree"
	"github.com/linkxzhou/gongx/packages/expr"
	"github.com/linkxzhou/gongx/packages/log"
	"github.com/linkxzhou/gongx/packages/utils"
)

type (
	selectorEntity struct {
		Name string
		Id   string
	}

	Queryer struct {
		Sql string
		// PREPARED (default), STATEMENT
		StatementType string
		Value         map[string]interface{}
	}

	KeyValue struct {
		Key   string
		Value interface{}
	}

	XmlParser struct {
		doc *etree.Document
	}
)

func (queryer *Queryer) Build(item *etree.Element, inputValue map[string]interface{}) error {
	var err error
	queryer.Value = make(map[string]interface{})
	parser := NewXmlParserBuild(inputValue, queryer.Value)
	queryer.StatementType = strings.ToUpper(item.SelectAttrValue("statementType", "PREPARED"))
	queryer.Sql, err = parser.Build(item)
	return err
}

func (xml *XmlParser) LoadFromBytes(bytes []byte) error {
	xml.doc = etree.NewDocument()
	xml.doc.ReadSettings.Permissive = true // not check attribute
	return xml.doc.ReadFromBytes(bytes)
}

func (xml *XmlParser) Query(id string, inputValue map[string]interface{}) (*Queryer, error) {
	q := &Queryer{}
	item := xml.doc.FindElement("./mapper/*[@id='" + id + "']")
	if item == nil {
		return q, errors.New("XML id \"" + id + "\" is not exists")
	}
	err := q.Build(item, inputValue)
	return q, err
}

func (xml *XmlParser) QueryElements(name string, inputValue map[string]interface{}) ([]*Queryer, error) {
	var qList []*Queryer
	items := xml.doc.FindElements("./mapper/" + name)
	for _, item := range items {
		q := &Queryer{}
		if err := q.Build(item, inputValue); err != nil {
			return qList, err
		} else {
			qList = append(qList, q)
		}
	}
	return qList, nil
}

var (
	ptnOutputVar     = regexp.MustCompile(`#{(.*?)}`)
	ptnOutputReplace = regexp.MustCompile(`\${(.*?)}`)
)

type XmlParserBuild struct {
	inputValue  map[string]interface{}
	outputValue map[string]interface{}
}

func NewXmlParserBuild(inputValue, outputValue map[string]interface{}) *XmlParserBuild {
	return &XmlParserBuild{
		inputValue:  inputValue,
		outputValue: outputValue,
	}
}

func (xml *XmlParserBuild) Build(element *etree.Element) (string, error) {
	var err error
	var builder strings.Builder
	for _, child := range element.Child {
		expression := ""
		switch reflect.TypeOf(child).String() {
		case "*etree.CharData":
			expression = xml.filter(strings.TrimSpace(child.(*etree.CharData).Data))
		case "*etree.Element":
			el := child.(*etree.Element)
			switch el.Tag {
			case "trim":
				if expression, err = xml.buildTrim(el); err != nil {
					break
				}
			case "where":
				expression = xml.buildWhere(el)
			case "set":
				expression = xml.buildSet(el)
			case "if":
				expression = xml.buildIf(el)
			case "space":
				expression = xml.buildSpace(el)
			}
		}
		if expression != "" {
			builder.WriteString(expression)
			builder.WriteString(" ")
		}
	}
	return strings.TrimSpace(builder.String()), err
}

func (xml *XmlParserBuild) buildTrim(el *etree.Element) (string, error) {
	expression, err := xml.Build(el)
	if err != nil {
		return "", err
	}
	if expression == "" {
		return expression, err
	}
	if str := el.SelectAttrValue("prefixOverrides", ""); str != "" {
		for _, tag := range strings.Split(str, "|") {
			expression = strings.TrimLeft(expression, strings.TrimSpace(tag))
		}
	}
	if str := el.SelectAttrValue("suffixOverrides", ""); str != "" {
		for _, tag := range strings.Split(str, "|") {
			expression = strings.TrimRight(expression, strings.TrimSpace(tag))
		}
	}
	if expression != "" {
		expression = el.SelectAttrValue("prefix", "") + expression + el.SelectAttrValue("suffix", "")
	}
	return expression, nil
}

func (xml *XmlParserBuild) buildWhere(el *etree.Element) string {
	wheres := xml.buildWhereOrSet(el)
	if len(wheres) > 0 {
		return "WHERE " + strings.Join(wheres, " AND ")
	}
	return ""
}

func (xml *XmlParserBuild) buildSet(el *etree.Element) string {
	sets := xml.buildWhereOrSet(el)
	if len(sets) > 0 {
		return "SET " + strings.Join(sets, ", ")
	}
	return ""
}

func (xml *XmlParserBuild) buildSpace(el *etree.Element) string {
	sets := xml.buildWhereOrSet(el)
	if len(sets) > 0 {
		return strings.Join(sets, " ")
	}
	return ""
}

func (xml *XmlParserBuild) buildWhereOrSet(el *etree.Element) []string {
	ifElements := el.FindElements("if")
	wheres := make([]string, len(ifElements))
	index := 0
	for _, el := range ifElements {
		str := strings.TrimSpace(el.SelectAttrValue("test", ""))
		if b, err := xml.parseTest(str, xml.inputValue); err == nil && b {
			wheres[index] = xml.filter(el.Text())
			index++
		} else {
			log.Error("if `", str, "` error: (", err, ")")
		}
	}
	return wheres[0:index]
}

func (xml *XmlParserBuild) buildIf(el *etree.Element) string {
	str := strings.TrimSpace(el.SelectAttrValue("test", ""))
	if b, err := xml.parseTest(str, xml.inputValue); err == nil && b {
		return xml.filter(el.Text())
	} else {
		log.Error("if `", str, "` error: (", err, ")")
	}
	return ""
}

func (xml *XmlParserBuild) filter(str string) string {
	str = strings.Replace(str, "&lt;", "<", -1)
	str = strings.Replace(str, "&gt;", ">", -1)
	str = ptnOutputVar.ReplaceAllStringFunc(str, func(s string) string {
		key := strings.TrimSpace(s[2 : len(s)-1])
		xml.outputValue[key] = xml.inputValue[key]
		return ":" + key
	})
	str = ptnOutputReplace.ReplaceAllStringFunc(str, func(s string) string {
		key := strings.TrimSpace(s[2 : len(s)-1])
		return utils.ToStr(xml.inputValue[key])
	})
	return strings.TrimSpace(str)
}

func (xml *XmlParserBuild) parseTest(str string, args map[string]interface{}) (bool, error) {
	if str == "" {
		return true, nil
	}
	p, err := expr.Eval(str, args, nil)
	if err != nil {
		return false, err
	}
	if p.IsBool() {
		return p.Bool(), nil
	}
	return false, nil
}
