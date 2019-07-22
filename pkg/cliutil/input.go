package cliutil

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/AlecAivazis/survey.v1"
)

func GetYesInput(msg string) (bool, error) {
	var yesAnswer string

	if err := GetStringInputDefault(
		msg,
		&yesAnswer,
		"N",
	); err != nil {
		return false, err
	}
	return strings.ToLower(yesAnswer) == "y", nil
}

func GetStringInputLazy(msgProvider func() string, value *string) error {
	return GetStringInputDefaultLazy(msgProvider, value, "")
}

func GetStringInput(msg string, value *string) error {
	return GetStringInputDefault(msg, value, "")
}

func GetStringInputDefaultLazy(msgProvider func() string, value *string, defaultValue string) error {
	prompt := &survey.Input{Message: msgProvider(), Default: defaultValue}
	if err := AskOne(prompt, value, nil); err != nil {
		return err
	}
	return nil
}

func GetStringInputDefault(msg string, value *string, defaultValue string) error {
	prompt := &survey.Input{Message: msg, Default: defaultValue}
	if err := AskOne(prompt, value, nil); err != nil {
		return err
	}
	return nil
}

func GetUint32Input(msg string, value *uint32) error {
	return GetUint32InputDefault(msg, value, 0)
}

func GetUint32InputDefault(msg string, value *uint32, defaultValue uint32) error {
	var strValue string
	prompt := &survey.Input{Message: msg, Default: strconv.Itoa(int(defaultValue))}
	if err := AskOne(prompt, &strValue, nil); err != nil {
		return err
	}
	val, err := strconv.Atoi(strValue)
	if err != nil {
		return err
	}
	*value = uint32(val)
	return nil
}

func GetBoolInput(msg string, value *bool) error {
	return GetBoolInputDefault(msg, value, false)
}

func GetBoolInputDefault(msg string, value *bool, defaultValue bool) error {
	var strValue string
	defaultValueStr := "N"
	if defaultValue {
		defaultValueStr = "y"
	}
	prompt := &survey.Input{Message: msg + " [y/N]: ", Default: defaultValueStr}
	if err := AskOne(prompt, &strValue, nil); err != nil {
		return err
	}
	*value = strings.ToLower(strValue) == "y"
	return nil
}

func GetStringSliceInputLazy(msgProvider func() string, value *[]string) error {
	for {
		var entry string
		if err := GetStringInputLazy(msgProvider, &entry); err != nil {
			return err
		}

		if entry == "" {
			return nil
		}
		*value = append(*value, entry)
	}
}

func GetStringSliceInput(msg string, value *[]string) error {
	for {
		var entry string
		if err := GetStringInput(msg, &entry); err != nil {
			return err
		}

		if entry == "" {
			return nil
		}
		*value = append(*value, entry)
	}
}

func ChooseFromList(message string, choice *string, options []string) error {
	if len(options) == 0 {
		return fmt.Errorf("No options to select from (for prompt: %v)", message)
	}

	question := &survey.Select{
		Message: message,
		Options: options,
	}

	if err := AskOne(question, choice, survey.Required); err != nil {
		// this should not error
		fmt.Println("error with input")
		return err
	}

	return nil
}

func ChooseBool(message string, target *bool) error {

	yes, no := "yes", "no"

	question := &survey.Select{
		Message: message,
		Options: []string{yes, no},
	}

	var choice string
	if err := AskOne(question, &choice, survey.Required); err != nil {
		return err
	}

	*target = choice == yes
	return nil
}

type JoinerData interface {
	Join() string
	ID() string
}
type JoinerDataSlice []JoinerData

func SelectJoinedData(message string, target *string, list []JoinerData) error {
	var optionsList []string
	for i, j := range list {
		// construct the options
		optionsList = append(optionsList, fmt.Sprintf("%v. %v", i, j.Join()))
	}
	question := &survey.Select{
		Message: message,
		Options: optionsList,
	}

	var choice string
	if err := AskOne(question, &choice, survey.Required); err != nil {
		return err
	}

	parts := strings.SplitN(choice, ".", 2)
	index, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}

	*target = list[index].ID()

	return nil
}

func EnsureCsv(message string, source string, target *[]string, staticMode bool) error {
	if staticMode && source == "" {
		return fmt.Errorf(message)
	}
	if !staticMode {
		if err := GetStringInput(message, &source); err != nil {
			return err
		}
	}
	parts := strings.Split(source, ",")
	*target = parts
	return nil
}

// Expected format of source: k1,v1,k2,v2
func EnsureKVCsv(message string, source string, target *map[string]string, staticMode bool) error {
	parts := []string{}
	EnsureCsv(message, source, &parts, staticMode)
	if len(parts) == 1 && parts[0] == "" {
		// case where user does not specify any values
		return nil
	}
	if len(parts)%2 != 0 {
		return fmt.Errorf("Must provide one key per value (received an odd sum)")
	}
	for i := 0; i < len(parts)/2; i++ {
		(*target)[parts[i*2]] = parts[i*2+1]
	}
	return nil
}
