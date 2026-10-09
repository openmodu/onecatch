package keeldesktop

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var templateToken = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
var customTemplateID = regexp.MustCompile(`^custom-[a-zA-Z0-9-]+$`)

func validateTemplates(items []promptTemplate) error {
	if len(items) > 100 {
		return fmt.Errorf("模板数量不能超过 100")
	}
	seen := map[string]bool{}
	for _, p := range items {
		if !customTemplateID.MatchString(p.ID) || seen[p.ID] || strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 120 || strings.TrimSpace(p.Body) == "" || utf8.RuneCountInString(p.Body) > 32000 || p.Builtin {
			return fmt.Errorf("模板格式无效：%s", p.ID)
		}
		seen[p.ID] = true
	}
	return nil
}
func expandTemplate(p promptTemplate, library []promptTemplate, resolve func(bool, string) (string, error)) (string, error) {
	tokens := 0
	var expand func(promptTemplate, []string) (string, error)
	expand = func(current promptTemplate, stack []string) (string, error) {
		if len(stack) >= 8 {
			return "", fmt.Errorf("模板引用深度超过限制")
		}
		for _, id := range stack {
			if id == current.ID {
				return "", fmt.Errorf("模板循环引用：%s", current.Name)
			}
		}
		if utf8.RuneCountInString(current.Body) > 32000 {
			return "", fmt.Errorf("模板内容过长")
		}
		matches := templateToken.FindAllStringSubmatchIndex(current.Body, -1)
		var out strings.Builder
		offset := 0
		for _, m := range matches {
			tokens++
			if tokens > 1000 {
				return "", fmt.Errorf("模板变量超过限制")
			}
			out.WriteString(current.Body[offset:m[0]])
			token := strings.TrimSpace(current.Body[m[2]:m[3]])
			value := ""
			var err error
			if strings.HasPrefix(token, "template:") {
				id := strings.TrimSpace(strings.TrimPrefix(token, "template:"))
				found := false
				for _, nested := range library {
					if nested.ID == id {
						found = true
						next := append(append([]string(nil), stack...), current.ID)
						value, err = expand(nested, next)
						break
					}
				}
				if !found {
					return "", fmt.Errorf("引用的模板不存在：%s", id)
				}
			} else if strings.HasPrefix(token, "arg:") && strings.TrimSpace(token[4:]) != "" {
				value, err = resolve(true, strings.TrimSpace(token[4:]))
			} else {
				switch token {
				case "selection", "project", "path", "task", "date":
					value, err = resolve(false, token)
				default:
					return "", fmt.Errorf("不支持的模板变量：%s", token)
				}
			}
			if err != nil {
				return "", err
			}
			out.WriteString(value)
			if out.Len() > 128000*4 {
				return "", fmt.Errorf("生成的指令过长")
			}
			offset = m[1]
		}
		out.WriteString(current.Body[offset:])
		if utf8.RuneCountInString(out.String()) > 128000 {
			return "", fmt.Errorf("生成的指令过长")
		}
		return out.String(), nil
	}
	return expand(p, nil)
}
func templateArguments(p promptTemplate, library []promptTemplate) ([]string, error) {
	args := []string{}
	seen := map[string]bool{}
	_, err := expandTemplate(p, library, func(argument bool, name string) (string, error) {
		if argument && !seen[name] {
			seen[name] = true
			args = append(args, name)
		}
		return "", nil
	})
	return args, err
}
func renderTemplate(p promptTemplate, library []promptTemplate, context, args map[string]string) (string, error) {
	return expandTemplate(p, library, func(argument bool, name string) (string, error) {
		values := context
		if argument {
			values = args
		}
		value := values[name]
		if strings.TrimSpace(value) == "" {
			if argument {
				return "", fmt.Errorf("请填写参数：%s", name)
			}
			if name == "selection" {
				return "", fmt.Errorf("先输入需要处理的材料")
			}
		}
		return value, nil
	})
}
