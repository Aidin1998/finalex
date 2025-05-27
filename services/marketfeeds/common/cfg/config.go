package cfg

import (
	"os"

	"github.com/spf13/viper"
)

func MustLoad[T any]() *T {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		panic("Couldn't load configuration, cannot start. Terminating. Error: " + err.Error())
	}

	//for _, k := range viper.AllKeys() {
	//value := viper.GetString(k)
	//if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
	//viper.Set(k, getEnvOrPanic(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")))
	//}
	//}

	var config T
	if err := viper.Unmarshal(&config); err != nil {
		panic("Failed to unmarshal config file: " + err.Error())
	}

	return &config
}

func getEnvOrPanic(env string) string {
	res := os.Getenv(env)
	if len(res) == 0 {
		panic("env variable not found:" + env)
	}
	return res
}
