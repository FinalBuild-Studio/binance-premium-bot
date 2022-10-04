package models

type EventMessage struct {
	Type    string
	Setting *ConfigSetting
	Message any
}
