package database

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/beevik/etree"
	"github.com/linkxzhou/goedge/packages/expr"
	"github.com/linkxzhou/goedge/packages/utils"
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
		var exprResult string
		var err error

		switch reflect.TypeOf(child).String() {
		case "*etree.CharData":
			exprResult = xml.filter(strings.TrimSpace(child.(*etree.CharData).Data))
		case "*etree.Element":
			el := child.(*etree.Element)
			switch el.Tag {
			case "trim":
				if exprResult, err = xml.buildTrim(el); err != nil {
					break
				}
			case "where":
				exprResult, err = xml.buildWhere(el)
			case "set":
				exprResult, err = xml.buildSet(el)
			case "if":
				exprResult, err = xml.buildIf(el)
			case "with":
				var joinStr = " " // default space string
				if attr := el.SelectAttr("join"); attr != nil {
					joinStr = attr.Value
				}
				exprResult, err = xml.buildWith(el, joinStr)
			}
		}

		if err != nil {
			return "", err
		}

		if exprResult != "" {
			builder.WriteString(exprResult)
			builder.WriteString(" ")
		}
	}
	return strings.TrimSpace(builder.String()), err
}

func (xml *XmlParserBuild) buildTrim(el *etree.Element) (string, error) {
	exprResult, err := xml.Build(el)
	if err != nil {
		return "", err
	}
	if exprResult == "" {
		return exprResult, err
	}
	if str := el.SelectAttrValue("prefixOverrides", ""); str != "" {
		for _, tag := range strings.Split(str, "|") {
			exprResult = strings.TrimLeft(exprResult, strings.TrimSpace(tag))
		}
	}
	if str := el.SelectAttrValue("suffixOverrides", ""); str != "" {
		for _, tag := range strings.Split(str, "|") {
			exprResult = strings.TrimRight(exprResult, strings.TrimSpace(tag))
		}
	}
	if exprResult != "" {
		exprResult = el.SelectAttrValue("prefix", "") + exprResult + el.SelectAttrValue("suffix", "")
	}
	return exprResult, nil
}

func (xml *XmlParserBuild) buildWhere(el *etree.Element) (string, error) {
	if wheres, err := xml.buildWhereOrSet(el); err == nil && len(wheres) > 0 {
		return "WHERE " + strings.Join(wheres, " AND "), nil
	} else {
		return "", err
	}
}

func (xml *XmlParserBuild) buildSet(el *etree.Element) (string, error) {
	if sets, err := xml.buildWhereOrSet(el); err == nil && len(sets) > 0 {
		return "SET " + strings.Join(sets, ", "), nil
	} else {
		return "", err
	}
}

func (xml *XmlParserBuild) buildWith(el *etree.Element, join string) (string, error) {
	if sets, err := xml.buildWhereOrSet(el); err == nil && len(sets) > 0 {
		return strings.Join(sets, join), err
	} else {
		return "", err
	}
}

func (xml *XmlParserBuild) buildWhereOrSet(el *etree.Element) ([]string, error) {
	ifElements := el.FindElements("if")
	wheres := make([]string, len(ifElements))
	index := 0
	for _, el := range ifElements {
		str := strings.TrimSpace(el.SelectAttrValue("test", ""))
		if b, err := xml.parseTest(str, xml.inputValue); err == nil && b {
			wheres[index] = xml.filter(el.Text())
			index++
		} else {
			return nil, err
		}
	}
	return wheres[0:index], nil
}

func (xml *XmlParserBuild) buildIf(el *etree.Element) (string, error) {
	str := strings.TrimSpace(el.SelectAttrValue("test", ""))
	if b, err := xml.parseTest(str, xml.inputValue); err == nil && b {
		return xml.filter(el.Text()), err
	} else {
		return "", err
	}
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
	// set var missing function
	expr.DefaultPool.SetOnVarMissing(func(varName string) (expr.Value, error) {
		return expr.NilValue, nil
	})
	p, err := expr.Eval(str, args, nil)
	if err != nil {
		return false, err
	}
	if p.IsBool() {
		return p.Bool(), nil
	}
	return false, nil
}
