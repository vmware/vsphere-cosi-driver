/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"sigs.k8s.io/vsphere-cosi-driver/pkg/cosi/service"
	cositypes "sigs.k8s.io/vsphere-cosi-driver/pkg/cosi/types"
)

func main() {
	log, err := newLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create logger: %v\n", err)
		os.Exit(1)
	}

	endpoint := os.Getenv(cositypes.EnvVarEndpoint)
	if endpoint == "" {
		log.Errorf("COSI endpoint cannot be empty. Please set the env variable %s.", cositypes.EnvVarEndpoint)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.NewDriver(log).Run(ctx, endpoint); err != nil {
		log.Errorf("exiting on error: %v", err)
		os.Exit(1)
	}
}

func newLogger() (*zap.SugaredLogger, error) {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	return logger.Sugar(), nil
}
