package lib

import (
	"fmt"
	"os"
)

func env(k string) (string, error) {
	v := os.Getenv(k)
	if v == "" {
		return "", fmt.Errorf("'%v' is empty", k)
	}
	return v, nil
}

func getDSNToDB(conn, dbname string) string {
	return fmt.Sprintf("%s%s?parseTime=true", conn, dbname)
}

func envDSNToServer() (string, error) {
	user, err := env("DB_USER")
	if err != nil {
		return "", fmt.Errorf("env error, %w", err)
	}

	pw, err := env("DB_PASS")
	if err != nil {
		return "", fmt.Errorf("env error, %w", err)
	}

	icn, err := env("INSTANCE_CONNECTION_NAME")
	if err != nil {
		return "", fmt.Errorf("env error, %w", err)
	}

	return fmt.Sprintf("%s:%s@unix(//cloudsql/%s)/", user, pw, icn), nil
}

func EnvDSNToDB() (string, error) {
	conn, err := envDSNToServer()
	if err != nil {
		return "", fmt.Errorf("envDSNToServer error, %w", err)
	}

	name, err := env("DB_NAME")
	if err != nil {
		return "", fmt.Errorf("env error, %w", err)
	}
	return getDSNToDB(conn, name), nil
}

func EnvGCSBucket() (string, error) {
	b, err := env("GCS_BUCKET")
	if err != nil {
		return "", fmt.Errorf("env error, %w", err)
	}
	return b, nil
}
