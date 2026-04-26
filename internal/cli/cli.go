package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"minimalist/internal/app"
)

func Run(args []string) error {
	a := app.New()
	if len(args) == 0 {
		if isTTY() {
			return a.Menu()
		}
		printUsage()
		return nil
	}
	switch args[0] {
	case "menu":
		return a.Menu()
	case "install-self":
		return a.InstallSelf()
	case "setup":
		return a.Setup()
	case "render-config":
		return a.RenderConfig()
	case "start":
		return a.Start()
	case "stop":
		return a.Stop()
	case "restart":
		return a.Restart()
	case "status":
		return a.Status()
	case "show-secret":
		return a.ShowSecret()
	case "healthcheck":
		return a.Healthcheck()
	case "runtime-audit":
		return a.RuntimeAudit()
	case "import-links":
		return a.ImportLinks()
	case "router-wizard":
		return a.RouterWizard()
	case "apply-rules":
		return a.ApplyRules()
	case "clear-rules":
		return a.ClearRules()
	case "nodes":
		return runNodes(a, args[1:])
	case "subscriptions":
		return runSubscriptions(a, args[1:])
	case "rules":
		return runRules(a, false, args[1:])
	case "acl":
		return runRules(a, true, args[1:])
	case "rules-repo":
		return runRulesRepo(a, args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runNodes(a *app.App, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: minimalist nodes list|rename|enable|disable|remove ...")
	}
	switch args[0] {
	case "list":
		return a.ListNodes()
	case "rename":
		if len(args) < 3 {
			return errors.New("usage: minimalist nodes rename <index> <new-name>")
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return a.RenameNode(index, args[2])
	case "enable":
		return toggleNode(a, args[1:], true)
	case "disable":
		return toggleNode(a, args[1:], false)
	case "remove":
		if len(args) < 2 {
			return errors.New("usage: minimalist nodes remove <index>")
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return a.RemoveNode(index)
	default:
		return fmt.Errorf("unknown nodes command: %s", args[0])
	}
}

func toggleNode(a *app.App, args []string, enabled bool) error {
	if len(args) < 1 {
		return errors.New("usage: minimalist nodes enable|disable <index>")
	}
	index, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	return a.SetNodeEnabled(index, enabled)
}

func runSubscriptions(a *app.App, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: minimalist subscriptions list|add|enable|disable|remove|update ...")
	}
	switch args[0] {
	case "list":
		return a.ListSubscriptions()
	case "add":
		if len(args) < 3 {
			return errors.New("usage: minimalist subscriptions add <name> <url>")
		}
		return a.AddSubscription(args[1], args[2], true)
	case "enable":
		return toggleSubscription(a, args[1:], true)
	case "disable":
		return toggleSubscription(a, args[1:], false)
	case "remove":
		if len(args) < 2 {
			return errors.New("usage: minimalist subscriptions remove <index>")
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return a.RemoveSubscription(index)
	case "update":
		return a.UpdateSubscriptions()
	default:
		return fmt.Errorf("unknown subscriptions command: %s", args[0])
	}
}

func toggleSubscription(a *app.App, args []string, enabled bool) error {
	if len(args) < 1 {
		return errors.New("usage: minimalist subscriptions enable|disable <index>")
	}
	index, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	return a.SetSubscriptionEnabled(index, enabled)
}

func runRules(a *app.App, acl bool, args []string) error {
	label := "rules"
	if acl {
		label = "acl"
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: minimalist %s list|add|remove ...", label)
	}
	switch args[0] {
	case "list":
		return a.ListRules(acl)
	case "add":
		if len(args) < 4 {
			return fmt.Errorf("usage: minimalist %s add <kind> <pattern> <target>", label)
		}
		return a.AddRule(acl, args[1], args[2], args[3])
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: minimalist %s remove <index>", label)
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return a.RemoveRule(acl, index)
	default:
		return fmt.Errorf("unknown %s command: %s", label, args[0])
	}
}

func runRulesRepo(a *app.App, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: minimalist rules-repo summary|entries|find|add|remove|remove-index ...")
	}
	switch args[0] {
	case "summary":
		return a.RulesRepoSummary()
	case "entries":
		if len(args) < 2 {
			return errors.New("usage: minimalist rules-repo entries <ruleset> [keyword]")
		}
		keyword := ""
		if len(args) > 2 {
			keyword = args[2]
		}
		return a.RulesRepoEntries(args[1], keyword)
	case "find":
		if len(args) < 2 {
			return errors.New("usage: minimalist rules-repo find <keyword>")
		}
		return a.RulesRepoFind(strings.Join(args[1:], " "))
	case "add":
		if len(args) < 3 {
			return errors.New("usage: minimalist rules-repo add <ruleset> <value>")
		}
		return a.RulesRepoAdd(args[1], args[2])
	case "remove":
		if len(args) < 3 {
			return errors.New("usage: minimalist rules-repo remove <ruleset> <value>")
		}
		return a.RulesRepoRemove(args[1], args[2])
	case "remove-index":
		if len(args) < 3 {
			return errors.New("usage: minimalist rules-repo remove-index <ruleset> <index>")
		}
		index, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		return a.RulesRepoRemoveIndex(args[1], index)
	default:
		return fmt.Errorf("unknown rules-repo command: %s", args[0])
	}
}

func printUsage() {
	fmt.Println(`minimalist commands:
  minimalist menu
  minimalist install-self
  minimalist setup
  minimalist render-config
  minimalist start|stop|restart
  minimalist status|show-secret|healthcheck|runtime-audit
  minimalist import-links
  minimalist router-wizard
  minimalist apply-rules|clear-rules
  minimalist nodes list|rename|enable|disable|remove
  minimalist subscriptions list|add|enable|disable|remove|update
  minimalist rules list|add|remove
  minimalist acl list|add|remove
  minimalist rules-repo summary|entries|find|add|remove|remove-index`)
}

func isTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
