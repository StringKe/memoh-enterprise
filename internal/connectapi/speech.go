package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/protobuf/types/known/structpb"

	audiopkg "github.com/memohai/memoh/internal/audio"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/models"
)

type speechAudioService interface {
	ListSpeechProviders(ctx context.Context) ([]audiopkg.SpeechProviderResponse, error)
	ListTranscriptionProviders(ctx context.Context) ([]audiopkg.SpeechProviderResponse, error)
	GetSpeechProvider(ctx context.Context, id string) (audiopkg.SpeechProviderResponse, error)
	ListSpeechMeta(ctx context.Context) []audiopkg.ProviderMetaResponse
	ListTranscriptionMeta(ctx context.Context) []audiopkg.ProviderMetaResponse
	FetchRemoteModels(ctx context.Context, providerID string) ([]audiopkg.ModelInfo, error)
	FetchRemoteTranscriptionModels(ctx context.Context, providerID string) ([]audiopkg.ModelInfo, error)
	ListSpeechModels(ctx context.Context) ([]audiopkg.SpeechModelResponse, error)
	ListSpeechModelsByProvider(ctx context.Context, providerID string) ([]audiopkg.SpeechModelResponse, error)
	ListTranscriptionModels(ctx context.Context) ([]audiopkg.TranscriptionModelResponse, error)
	ListTranscriptionModelsByProvider(ctx context.Context, providerID string) ([]audiopkg.TranscriptionModelResponse, error)
	GetSpeechModel(ctx context.Context, id string) (audiopkg.SpeechModelResponse, error)
	GetTranscriptionModel(ctx context.Context, id string) (audiopkg.TranscriptionModelResponse, error)
	UpdateSpeechModel(ctx context.Context, id string, req audiopkg.UpdateSpeechModelRequest) (audiopkg.SpeechModelResponse, error)
	UpdateTranscriptionModel(ctx context.Context, id string, req audiopkg.UpdateSpeechModelRequest) (audiopkg.TranscriptionModelResponse, error)
	GetSpeechModelCapabilities(ctx context.Context, modelID string) (*audiopkg.ModelCapabilities, error)
	GetTranscriptionModelCapabilities(ctx context.Context, modelID string) (*audiopkg.ModelCapabilities, error)
	Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error)
	Transcribe(ctx context.Context, modelID string, audio []byte, filename string, contentType string, overrideCfg map[string]any) (*sdk.TranscriptionResult, error)
}

type speechModelAdmin interface {
	Create(ctx context.Context, req models.AddRequest) (models.AddResponse, error)
}

type SpeechService struct {
	privatev1connect.UnimplementedSpeechServiceHandler

	audio  speechAudioService
	models speechModelAdmin
}

func NewSpeechService(audioService *audiopkg.Service, modelsService *models.Service) *SpeechService {
	return &SpeechService{
		audio:  audioService,
		models: modelsService,
	}
}

func NewSpeechHandler(service *SpeechService) Handler {
	path, handler := privatev1connect.NewSpeechServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *SpeechService) ListSpeechProviders(ctx context.Context, _ *connect.Request[privatev1.ListSpeechProvidersRequest]) (*connect.Response[privatev1.ListSpeechProvidersResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	items, err := s.audio.ListSpeechProviders(ctx)
	if err != nil {
		return nil, speechConnectError(err)
	}
	out := make([]*privatev1.SpeechProvider, 0, len(items))
	for _, item := range items {
		out = append(out, speechProviderToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSpeechProvidersResponse{Providers: out}), nil
}

func (s *SpeechService) GetSpeechProvider(ctx context.Context, req *connect.Request[privatev1.GetSpeechProviderRequest]) (*connect.Response[privatev1.GetSpeechProviderResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.audio.GetSpeechProvider(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetSpeechProviderResponse{Provider: speechProviderToProto(provider)}), nil
}

func (s *SpeechService) ListSpeechProviderMeta(ctx context.Context, _ *connect.Request[privatev1.ListSpeechProviderMetaRequest]) (*connect.Response[privatev1.ListSpeechProviderMetaResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	items := s.audio.ListSpeechMeta(ctx)
	out := make([]*privatev1.SpeechProviderMeta, 0, len(items))
	for _, item := range items {
		out = append(out, speechProviderMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSpeechProviderMetaResponse{Providers: out}), nil
}

func (s *SpeechService) ImportSpeechProviderModels(ctx context.Context, req *connect.Request[privatev1.ImportSpeechProviderModelsRequest]) (*connect.Response[privatev1.ImportSpeechProviderModelsResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	if s.models == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("models service not configured"))
	}
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}

	remoteModels, err := s.audio.FetchRemoteModels(ctx, providerID)
	if err != nil {
		return nil, speechConnectError(fmt.Errorf("fetch remote speech models: %w", err))
	}

	allowed := stringSet(req.Msg.GetModelIds())
	out := make([]*privatev1.SpeechModel, 0, len(remoteModels))
	for _, remote := range remoteModels {
		if len(allowed) > 0 {
			if _, ok := allowed[remote.ID]; !ok {
				continue
			}
		}

		name := strings.TrimSpace(remote.Name)
		if name == "" {
			name = remote.ID
		}
		created, err := s.models.Create(ctx, models.AddRequest{
			ModelID:    remote.ID,
			Name:       name,
			ProviderID: providerID,
			Type:       models.ModelTypeSpeech,
			Config:     models.ModelConfig{},
		})
		if err != nil {
			if errors.Is(err, models.ErrModelIDAlreadyExists) {
				continue
			}
			return nil, speechConnectError(err)
		}
		model, err := s.audio.GetSpeechModel(ctx, created.ID)
		if err != nil {
			return nil, speechConnectError(err)
		}
		out = append(out, speechModelToProto(model))
	}

	return connect.NewResponse(&privatev1.ImportSpeechProviderModelsResponse{Models: out}), nil
}

func (s *SpeechService) ListSpeechModels(ctx context.Context, req *connect.Request[privatev1.ListSpeechModelsRequest]) (*connect.Response[privatev1.ListSpeechModelsResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	var (
		items []audiopkg.SpeechModelResponse
		err   error
	)
	if providerID == "" {
		items, err = s.audio.ListSpeechModels(ctx)
	} else {
		items, err = s.audio.ListSpeechModelsByProvider(ctx, providerID)
	}
	if err != nil {
		return nil, speechConnectError(err)
	}
	out := make([]*privatev1.SpeechModel, 0, len(items))
	for _, item := range items {
		out = append(out, speechModelToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSpeechModelsResponse{Models: out}), nil
}

func (s *SpeechService) GetSpeechModel(ctx context.Context, req *connect.Request[privatev1.GetSpeechModelRequest]) (*connect.Response[privatev1.GetSpeechModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	model, err := s.audio.GetSpeechModel(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetSpeechModelResponse{Model: speechModelToProto(model)}), nil
}

func (s *SpeechService) UpdateSpeechModel(ctx context.Context, req *connect.Request[privatev1.UpdateSpeechModelRequest]) (*connect.Response[privatev1.UpdateSpeechModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	update := audiopkg.UpdateSpeechModelRequest{
		Config: structToMap(req.Msg.GetMetadata()),
	}
	if req.Msg.DisplayName != nil {
		name := strings.TrimSpace(req.Msg.GetDisplayName())
		update.Name = &name
	}
	model, err := s.audio.UpdateSpeechModel(ctx, id, update)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateSpeechModelResponse{Model: speechModelToProto(model)}), nil
}

func (s *SpeechService) GetSpeechModelCapabilities(ctx context.Context, req *connect.Request[privatev1.GetSpeechModelCapabilitiesRequest]) (*connect.Response[privatev1.GetSpeechModelCapabilitiesResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	caps, err := s.audio.GetSpeechModelCapabilities(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetSpeechModelCapabilitiesResponse{Capabilities: speechCapabilitiesToStruct(caps)}), nil
}

func (s *SpeechService) TestSpeechModel(ctx context.Context, req *connect.Request[privatev1.TestSpeechModelRequest]) (*connect.Response[privatev1.TestSpeechModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	text := strings.TrimSpace(req.Msg.GetText())
	if text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text is required"))
	}
	const maxTestTextLen = 500
	if len([]rune(text)) > maxTestTextLen {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text too long, max 500 characters"))
	}
	_, contentType, err := s.audio.Synthesize(ctx, id, text, nil)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.TestSpeechModelResponse{
		Ok:      true,
		Message: contentType,
	}), nil
}

func (s *SpeechService) SynthesizeBotSpeech(ctx context.Context, req *connect.Request[privatev1.SynthesizeBotSpeechRequest]) (*connect.Response[privatev1.SynthesizeBotSpeechResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	modelID := strings.TrimSpace(req.Msg.GetModelId())
	if modelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("model_id is required"))
	}
	text := strings.TrimSpace(req.Msg.GetText())
	if text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text is required"))
	}
	const maxSynthesizeTextLen = 500
	if len([]rune(text)) > maxSynthesizeTextLen {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("text too long, max 500 characters"))
	}
	audio, contentType, err := s.audio.Synthesize(ctx, modelID, text, nil)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.SynthesizeBotSpeechResponse{
		ContentType: contentType,
		Audio:       audio,
	}), nil
}

func (s *SpeechService) ListTranscriptionProviders(ctx context.Context, _ *connect.Request[privatev1.ListTranscriptionProvidersRequest]) (*connect.Response[privatev1.ListTranscriptionProvidersResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	items, err := s.audio.ListTranscriptionProviders(ctx)
	if err != nil {
		return nil, speechConnectError(err)
	}
	out := make([]*privatev1.SpeechProvider, 0, len(items))
	for _, item := range items {
		out = append(out, speechProviderToProto(item))
	}
	return connect.NewResponse(&privatev1.ListTranscriptionProvidersResponse{Providers: out}), nil
}

func (s *SpeechService) GetTranscriptionProvider(ctx context.Context, req *connect.Request[privatev1.GetTranscriptionProviderRequest]) (*connect.Response[privatev1.GetTranscriptionProviderResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.audio.GetSpeechProvider(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetTranscriptionProviderResponse{Provider: speechProviderToProto(provider)}), nil
}

func (s *SpeechService) ListTranscriptionProviderMeta(ctx context.Context, _ *connect.Request[privatev1.ListTranscriptionProviderMetaRequest]) (*connect.Response[privatev1.ListTranscriptionProviderMetaResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	items := s.audio.ListTranscriptionMeta(ctx)
	out := make([]*privatev1.SpeechProviderMeta, 0, len(items))
	for _, item := range items {
		out = append(out, speechProviderMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListTranscriptionProviderMetaResponse{Providers: out}), nil
}

func (s *SpeechService) ImportTranscriptionProviderModels(ctx context.Context, req *connect.Request[privatev1.ImportTranscriptionProviderModelsRequest]) (*connect.Response[privatev1.ImportTranscriptionProviderModelsResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	if s.models == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("models service not configured"))
	}
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}

	remoteModels, err := s.audio.FetchRemoteTranscriptionModels(ctx, providerID)
	if err != nil {
		return nil, speechConnectError(fmt.Errorf("fetch remote transcription models: %w", err))
	}

	allowed := stringSet(req.Msg.GetModelIds())
	out := make([]*privatev1.SpeechModel, 0, len(remoteModels))
	for _, remote := range remoteModels {
		if len(allowed) > 0 {
			if _, ok := allowed[remote.ID]; !ok {
				continue
			}
		}
		name := strings.TrimSpace(remote.Name)
		if name == "" {
			name = remote.ID
		}
		created, err := s.models.Create(ctx, models.AddRequest{
			ModelID:    remote.ID,
			Name:       name,
			ProviderID: providerID,
			Type:       models.ModelTypeTranscription,
			Config:     models.ModelConfig{},
		})
		if err != nil {
			if errors.Is(err, models.ErrModelIDAlreadyExists) {
				continue
			}
			return nil, speechConnectError(err)
		}
		model, err := s.audio.GetTranscriptionModel(ctx, created.ID)
		if err != nil {
			return nil, speechConnectError(err)
		}
		out = append(out, transcriptionModelToProto(model))
	}

	return connect.NewResponse(&privatev1.ImportTranscriptionProviderModelsResponse{Models: out}), nil
}

func (s *SpeechService) ListTranscriptionModels(ctx context.Context, req *connect.Request[privatev1.ListTranscriptionModelsRequest]) (*connect.Response[privatev1.ListTranscriptionModelsResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	var (
		items []audiopkg.TranscriptionModelResponse
		err   error
	)
	if providerID == "" {
		items, err = s.audio.ListTranscriptionModels(ctx)
	} else {
		items, err = s.audio.ListTranscriptionModelsByProvider(ctx, providerID)
	}
	if err != nil {
		return nil, speechConnectError(err)
	}
	out := make([]*privatev1.SpeechModel, 0, len(items))
	for _, item := range items {
		out = append(out, transcriptionModelToProto(item))
	}
	return connect.NewResponse(&privatev1.ListTranscriptionModelsResponse{Models: out}), nil
}

func (s *SpeechService) GetTranscriptionModel(ctx context.Context, req *connect.Request[privatev1.GetTranscriptionModelRequest]) (*connect.Response[privatev1.GetTranscriptionModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	model, err := s.audio.GetTranscriptionModel(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetTranscriptionModelResponse{Model: transcriptionModelToProto(model)}), nil
}

func (s *SpeechService) UpdateTranscriptionModel(ctx context.Context, req *connect.Request[privatev1.UpdateTranscriptionModelRequest]) (*connect.Response[privatev1.UpdateTranscriptionModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	update := audiopkg.UpdateSpeechModelRequest{
		Config: structToMap(req.Msg.GetMetadata()),
	}
	if req.Msg.DisplayName != nil {
		name := strings.TrimSpace(req.Msg.GetDisplayName())
		update.Name = &name
	}
	model, err := s.audio.UpdateTranscriptionModel(ctx, id, update)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateTranscriptionModelResponse{Model: transcriptionModelToProto(model)}), nil
}

func (s *SpeechService) GetTranscriptionModelCapabilities(ctx context.Context, req *connect.Request[privatev1.GetTranscriptionModelCapabilitiesRequest]) (*connect.Response[privatev1.GetTranscriptionModelCapabilitiesResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	caps, err := s.audio.GetTranscriptionModelCapabilities(ctx, id)
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetTranscriptionModelCapabilitiesResponse{Capabilities: speechCapabilitiesToStruct(caps)}), nil
}

func (s *SpeechService) TestTranscriptionModel(ctx context.Context, req *connect.Request[privatev1.TestTranscriptionModelRequest]) (*connect.Response[privatev1.TestTranscriptionModelResponse], error) {
	if s.audio == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("speech service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	audio := req.Msg.GetAudio()
	if len(audio) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("audio is required"))
	}
	result, err := s.audio.Transcribe(ctx, id, audio, req.Msg.GetFilename(), req.Msg.GetContentType(), structToMap(req.Msg.GetConfig()))
	if err != nil {
		return nil, speechConnectError(err)
	}
	return connect.NewResponse(&privatev1.TestTranscriptionModelResponse{
		Ok:              true,
		Message:         result.Text,
		Text:            result.Text,
		Language:        result.Language,
		DurationSeconds: result.DurationSeconds,
		Metadata:        mapToStruct(result.ProviderMetadata),
	}), nil
}

func speechProviderToProto(provider audiopkg.SpeechProviderResponse) *privatev1.SpeechProvider {
	return &privatev1.SpeechProvider{
		Id:      provider.ID,
		Name:    provider.Name,
		Type:    provider.ClientType,
		Enabled: provider.Enable,
		Config:  mapToStruct(provider.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(provider.CreatedAt),
			UpdatedAt: timeToProto(provider.UpdatedAt),
		},
	}
}

func speechModelToProto(model audiopkg.SpeechModelResponse) *privatev1.SpeechModel {
	return &privatev1.SpeechModel{
		Id:           model.ID,
		ProviderId:   model.ProviderID,
		ModelId:      model.ModelID,
		DisplayName:  firstNonEmptyString(model.Name, model.ModelID),
		Enabled:      true,
		Capabilities: mapToStruct(map[string]any{}),
		Metadata:     mapToStruct(model.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(model.CreatedAt),
			UpdatedAt: timeToProto(model.UpdatedAt),
		},
	}
}

func transcriptionModelToProto(model audiopkg.TranscriptionModelResponse) *privatev1.SpeechModel {
	return &privatev1.SpeechModel{
		Id:           model.ID,
		ProviderId:   model.ProviderID,
		ModelId:      model.ModelID,
		DisplayName:  firstNonEmptyString(model.Name, model.ModelID),
		Enabled:      true,
		Capabilities: mapToStruct(map[string]any{}),
		Metadata:     mapToStruct(model.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(model.CreatedAt),
			UpdatedAt: timeToProto(model.UpdatedAt),
		},
	}
}

func speechProviderMetaToProto(meta audiopkg.ProviderMetaResponse) *privatev1.SpeechProviderMeta {
	return &privatev1.SpeechProviderMeta{
		Type:        meta.Provider,
		DisplayName: meta.DisplayName,
		Schema:      configSchemaToStruct(meta.ConfigSchema),
	}
}

func configSchemaToStruct(schema audiopkg.ConfigSchema) *structpb.Struct {
	return valueToStruct(schema)
}

func speechCapabilitiesToStruct(caps *audiopkg.ModelCapabilities) *structpb.Struct {
	if caps == nil {
		return mapToStruct(map[string]any{})
	}
	return valueToStruct(caps)
}

func valueToStruct(value any) *structpb.Struct {
	payload, err := json.Marshal(value)
	if err != nil {
		return mapToStruct(map[string]any{})
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return mapToStruct(map[string]any{})
	}
	return mapToStruct(out)
}

func speechConnectError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, models.ErrModelIDAlreadyExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, models.ErrModelIDAmbiguous):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case strings.Contains(err.Error(), "not found"):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "too long"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
