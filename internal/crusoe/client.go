package crusoe

import (
	"context"
	"crusoe-registry-pruner/internal/crusoe/config"
	"fmt"
	"io"
	"log/slog"

	"github.com/antihax/optional"
	"github.com/crusoecloud/client-go/auth"
	"github.com/crusoecloud/client-go/swagger/v1alpha5"
)

const (
	pageSize  = 100
	pageLimit = 1000
)

type Client struct {
	apiClient *swagger.APIClient
	logger    *slog.Logger
	dryRun    bool
	project   string
}

func NewClient(
	cfg *config.Crusoe,
	logger *slog.Logger,
) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		logger:  logger,
		dryRun:  cfg.Pruner.DryRun,
		project: cfg.ProjectId.String(),
		apiClient: auth.NewAuthenticatedAPIClient(
			string(cfg.AccessKey),
			string(cfg.SecretKey),
		),
	}, nil
}

func (client *Client) GetRepositories(
	ctx context.Context,
) ([]swagger.Repository, error) {
	client.logger.Info("getting repositories")
	repositories, err := collectPages(func(page int32) ([]swagger.Repository, error) {
		return client.getRepositories(ctx, page)
	})

	if err != nil {
		return nil, fmt.Errorf("listing repositories in project %s: %w", client.project, err)
	}

	if len(repositories) == 0 {
		client.logger.Info("no repositories found")
	}

	return repositories, nil
}

func (client *Client) getRepositories(
	ctx context.Context,
	page int32,
) ([]swagger.Repository, error) {
	options := &swagger.CcrApiListCcrRepositoriesOpts{
		Page:      optional.NewInt32(page),
		PageSize:  optional.NewInt32(pageSize),
		SortBy:    optional.NewString("created_at"),
		SortOrder: optional.NewString("asc"),
	}
	repositories, response, err := client.apiClient.CcrApi.ListCcrRepositories(
		ctx, client.project, options,
	)
	if response != nil {
		defer client.cleanUpResponseBody(response.Body)
	}
	if err != nil {
		return nil, err
	}
	return repositories.Items, nil
}

func (client *Client) GetImages(
	ctx context.Context,
	repository swagger.Repository,
) ([]swagger.CcrImage, error) {
	group := slog.Group(
		"repository",
		slog.String("id", repository.Id),
		slog.String("name", repository.Name),
		slog.String("location", repository.Location),
	)

	client.logger.Info("getting images", group)
	images, err := collectPages(func(page int32) ([]swagger.CcrImage, error) {
		return client.getImages(ctx, repository, page)
	})

	if err != nil {
		return nil, fmt.Errorf("listing images in repository %s: %w", repository.Name, err)
	}

	if len(images) == 0 {
		client.logger.Info("no images found", group)
	}

	return images, nil
}

func (client *Client) getImages(
	ctx context.Context,
	repository swagger.Repository,
	page int32,
) ([]swagger.CcrImage, error) {
	options := &swagger.CcrApiListCcrImagesOpts{
		Location:  optional.NewString(repository.Location),
		Page:      optional.NewInt32(page),
		PageSize:  optional.NewInt32(pageSize),
		SortBy:    optional.NewString("updated_at"),
		SortOrder: optional.NewString("asc"),
	}
	images, response, err := client.apiClient.CcrApi.ListCcrImages(
		ctx, client.project, repository.Id, options,
	)
	if response != nil {
		defer client.cleanUpResponseBody(response.Body)
	}
	if err != nil {
		return nil, err
	}
	return images.Items, nil
}

func (client *Client) GetManifests(
	ctx context.Context,
	repository swagger.Repository,
	image swagger.CcrImage,
) ([]swagger.Manifest, error) {
	group := slog.Group(
		"repository",
		slog.String("id", repository.Id),
		slog.String("name", repository.Name),
		slog.String("location", repository.Location),
		slog.Group(
			"image",
			slog.String("name", image.Name),
		),
	)

	client.logger.Info("getting manifests", group)
	manifests, err := collectPages(func(page int32) ([]swagger.Manifest, error) {
		return client.getManifests(ctx, repository, image, page)
	})

	if err != nil {
		return nil, fmt.Errorf(
			"listing manifests in image %s/%s: %w", repository.Name, image.Name, err,
		)
	}

	if len(manifests) == 0 {
		client.logger.Info("no manifests found", group)
	}

	return manifests, nil
}

func (client *Client) getManifests(
	ctx context.Context,
	repository swagger.Repository,
	image swagger.CcrImage,
	page int32,
) ([]swagger.Manifest, error) {
	options := &swagger.CcrApiListCcrManifestsOpts{
		Page:      optional.NewInt32(page),
		PageSize:  optional.NewInt32(pageSize),
		SortBy:    optional.NewString("pulled_at"),
		SortOrder: optional.NewString("asc"),
	}
	manifests, response, err := client.apiClient.CcrApi.ListCcrManifests(
		ctx, client.project, repository.Id, image.Name, options,
	)
	if response != nil {
		defer client.cleanUpResponseBody(response.Body)
	}
	if err != nil {
		return nil, err
	}
	return manifests.Items, nil
}

func (client *Client) DeleteImage(
	ctx context.Context,
	repository swagger.Repository,
	image swagger.CcrImage,
) error {
	group := slog.Group(
		"repository",
		slog.String("id", repository.Id),
		slog.String("name", repository.Name),
		slog.String("location", repository.Location),
		slog.Group(
			"image",
			slog.String("name", image.Name),
		),
	)

	if client.dryRun {
		client.logger.Info("would delete image", group)
		return nil
	}

	options := &swagger.CcrApiDeleteCcrImageOpts{
		Location: optional.NewString(repository.Location),
	}
	response, err := client.apiClient.CcrApi.DeleteCcrImage(
		ctx, client.project, repository.Id, image.Name, options,
	)
	if response != nil {
		defer client.cleanUpResponseBody(response.Body)
	}
	if err != nil {
		return fmt.Errorf("deleting image %s/%s: %w", repository.Name, image.Name, err)
	}
	client.logger.Info("image deleted", group)
	return nil
}

func (client *Client) DeleteManifest(
	ctx context.Context,
	repository swagger.Repository,
	image swagger.CcrImage,
	manifest swagger.Manifest,
) error {
	group := slog.Group(
		"repository",
		slog.String("id", repository.Id),
		slog.String("name", repository.Name),
		slog.String("location", repository.Location),
		slog.Group(
			"image",
			slog.String("name", image.Name),
			slog.Group(
				"manifest",
				slog.String("digest", manifest.Digest),
				slog.String("bytes", manifest.SizeBytes),
			),
		),
	)

	if client.dryRun {
		client.logger.Info("would delete manifest", group)
		return nil
	}

	options := &swagger.CcrApiDeleteCcrManifestOpts{
		Location: optional.NewString(repository.Location),
		Digest:   optional.NewString(manifest.Digest),
	}
	response, err := client.apiClient.CcrApi.DeleteCcrManifest(
		ctx, client.project, repository.Id, image.Name, options,
	)
	if response != nil {
		defer client.cleanUpResponseBody(response.Body)
	}
	if err != nil {
		return fmt.Errorf(
			"deleting manifest %s/%s@%s: %w",
			repository.Name, image.Name, manifest.Digest, err,
		)
	}
	client.logger.Info("manifest deleted", group)
	return nil
}

func (client *Client) cleanUpResponseBody(closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		client.logger.Error("failed to close response body", "error", err)
	}
}

func collectPages[T any](fetch func(page int32) ([]T, error)) ([]T, error) {
	var result []T
	for page := int32(1); page <= pageLimit; page++ {
		results, err := fetch(page)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return result, nil
		}
		result = append(result, results...)
	}
	return nil, fmt.Errorf("exceeded page limit of %d pages at %d per page", pageLimit, pageSize)
}
