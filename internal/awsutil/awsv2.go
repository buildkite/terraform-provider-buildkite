package awsutil

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

// GetConfigV2 creates a new AWS SDK v2 config that uses the current region from
// IMDS, if not otherwise provided.
func GetConfigV2(ctx context.Context, optFns ...func(*config.LoadOptions) error) (cfg aws.Config, err error) {
	cfg, err = config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return cfg, fmt.Errorf("error loading default config: %w", err)
	}

	// If no region was provided, try to get it from IMDS
	if cfg.Region == "" {
		imdsClient := imds.NewFromConfig(cfg)
		region, err := imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
		if err == nil && region.Region != "" {
			cfg.Region = region.Region
		}
	}

	return cfg, nil
}
