package app

import "fmt"

func operatorActionError(problem string, cause error, next, doc string) error {
	if doc == "" {
		return fmt.Errorf("问题: %s; 原因: %v; 下一步: %s", problem, cause, next)
	}
	return fmt.Errorf("问题: %s; 原因: %v; 下一步: %s; 文档: %s", problem, cause, next, doc)
}
