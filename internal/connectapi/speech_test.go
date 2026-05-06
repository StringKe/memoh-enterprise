package connectapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/protobuf/types/known/structpb"

	audiopkg "github.com/memohai/memoh/internal/audio"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/models"
)

type fakeSpeechAudioService struct {
	providers        []audiopkg.SpeechProviderResponse
	meta             []audiopkg.ProviderMetaResponse
	remoteModels     []audiopkg.ModelInfo
	models           []audiopkg.SpeechModelResponse
	transModels      []audiopkg.TranscriptionModelResponse
	createdModels    map[string]audiopkg.SpeechModelResponse
	updateReq        audiopkg.UpdateSpeechModelRequest
	synthesizeText   string
	synthesizeModel  string
	getProviderErr   error
	getModelErr      error
	updateModelErr   error
	capabilitiesErr  error
	synthesizeErr    error
	fetchRemoteErr   error
	listProvidersErr error
}

func (f *fakeSpeechAudioService) ListSpeechProviders(context.Context) ([]audiopkg.SpeechProviderResponse, error) {
	return f.providers, f.listProvidersErr
}

func (f *fakeSpeechAudioService) ListTranscriptionProviders(context.Context) ([]audiopkg.SpeechProviderResponse, error) {
	return f.providers, f.listProvidersErr
}

func (f *fakeSpeechAudioService) GetSpeechProvider(_ context.Context, id string) (audiopkg.SpeechProviderResponse, error) {
	if f.getProviderErr != nil {
		return audiopkg.SpeechProviderResponse{}, f.getProviderErr
	}
	for _, provider := range f.providers {
		if provider.ID == id {
			return provider, nil
		}
	}
	return audiopkg.SpeechProviderResponse{}, errors.New("speech provider not found")
}

func (f *fakeSpeechAudioService) ListSpeechMeta(context.Context) []audiopkg.ProviderMetaResponse {
	return f.meta
}

func (f *fakeSpeechAudioService) ListTranscriptionMeta(context.Context) []audiopkg.ProviderMetaResponse {
	return f.meta
}

func (f *fakeSpeechAudioService) FetchRemoteModels(context.Context, string) ([]audiopkg.ModelInfo, error) {
	return f.remoteModels, f.fetchRemoteErr
}

func (f *fakeSpeechAudioService) FetchRemoteTranscriptionModels(context.Context, string) ([]audiopkg.ModelInfo, error) {
	return f.remoteModels, f.fetchRemoteErr
}

func (f *fakeSpeechAudioService) ListSpeechModels(context.Context) ([]audiopkg.SpeechModelResponse, error) {
	return f.models, nil
}

func (f *fakeSpeechAudioService) ListTranscriptionModels(context.Context) ([]audiopkg.TranscriptionModelResponse, error) {
	return f.transModels, nil
}

func (f *fakeSpeechAudioService) ListSpeechModelsByProvider(_ context.Context, providerID string) ([]audiopkg.SpeechModelResponse, error) {
	out := make([]audiopkg.SpeechModelResponse, 0, len(f.models))
	for _, model := range f.models {
		if model.ProviderID == providerID {
			out = append(out, model)
		}
	}
	return out, nil
}

func (f *fakeSpeechAudioService) ListTranscriptionModelsByProvider(_ context.Context, providerID string) ([]audiopkg.TranscriptionModelResponse, error) {
	out := make([]audiopkg.TranscriptionModelResponse, 0, len(f.transModels))
	for _, model := range f.transModels {
		if model.ProviderID == providerID {
			out = append(out, model)
		}
	}
	return out, nil
}

func (f *fakeSpeechAudioService) GetSpeechModel(_ context.Context, id string) (audiopkg.SpeechModelResponse, error) {
	if f.getModelErr != nil {
		return audiopkg.SpeechModelResponse{}, f.getModelErr
	}
	if f.createdModels != nil {
		if model, ok := f.createdModels[id]; ok {
			return model, nil
		}
	}
	for _, model := range f.models {
		if model.ID == id {
			return model, nil
		}
	}
	return audiopkg.SpeechModelResponse{}, errors.New("speech model not found")
}

func (f *fakeSpeechAudioService) GetTranscriptionModel(_ context.Context, id string) (audiopkg.TranscriptionModelResponse, error) {
	if f.getModelErr != nil {
		return audiopkg.TranscriptionModelResponse{}, f.getModelErr
	}
	for _, model := range f.transModels {
		if model.ID == id {
			return model, nil
		}
	}
	return audiopkg.TranscriptionModelResponse{
		ID:         id,
		ModelID:    "stt-1",
		Name:       "STT 1",
		ProviderID: "provider-1",
	}, nil
}

func (f *fakeSpeechAudioService) UpdateSpeechModel(_ context.Context, id string, req audiopkg.UpdateSpeechModelRequest) (audiopkg.SpeechModelResponse, error) {
	f.updateReq = req
	if f.updateModelErr != nil {
		return audiopkg.SpeechModelResponse{}, f.updateModelErr
	}
	return audiopkg.SpeechModelResponse{
		ID:         id,
		ModelID:    "tts-1",
		Name:       valueOrEmpty(req.Name),
		ProviderID: "provider-1",
		Config:     req.Config,
	}, nil
}

func (f *fakeSpeechAudioService) UpdateTranscriptionModel(_ context.Context, id string, req audiopkg.UpdateSpeechModelRequest) (audiopkg.TranscriptionModelResponse, error) {
	f.updateReq = req
	if f.updateModelErr != nil {
		return audiopkg.TranscriptionModelResponse{}, f.updateModelErr
	}
	return audiopkg.TranscriptionModelResponse{
		ID:         id,
		ModelID:    "stt-1",
		Name:       valueOrEmpty(req.Name),
		ProviderID: "provider-1",
		Config:     req.Config,
	}, nil
}

func (f *fakeSpeechAudioService) GetSpeechModelCapabilities(context.Context, string) (*audiopkg.ModelCapabilities, error) {
	if f.capabilitiesErr != nil {
		return nil, f.capabilitiesErr
	}
	return &audiopkg.ModelCapabilities{
		Formats: []string{"mp3"},
		Voices:  []audiopkg.VoiceInfo{{ID: "alloy", Name: "Alloy", Lang: "en"}},
	}, nil
}

func (f *fakeSpeechAudioService) GetTranscriptionModelCapabilities(context.Context, string) (*audiopkg.ModelCapabilities, error) {
	if f.capabilitiesErr != nil {
		return nil, f.capabilitiesErr
	}
	return &audiopkg.ModelCapabilities{
		Formats: []string{"wav"},
	}, nil
}

func (f *fakeSpeechAudioService) Synthesize(_ context.Context, modelID string, text string, _ map[string]any) ([]byte, string, error) {
	f.synthesizeModel = modelID
	f.synthesizeText = text
	if f.synthesizeErr != nil {
		return nil, "", f.synthesizeErr
	}
	return []byte("audio"), "audio/mpeg", nil
}

func (*fakeSpeechAudioService) Transcribe(context.Context, string, []byte, string, string, map[string]any) (*sdk.TranscriptionResult, error) {
	return &sdk.TranscriptionResult{
		Text:     "hello",
		Language: "en",
	}, nil
}

type fakeSpeechModelAdmin struct {
	createCalls []models.AddRequest
	createErr   error
}

func (f *fakeSpeechModelAdmin) Create(_ context.Context, req models.AddRequest) (models.AddResponse, error) {
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return models.AddResponse{}, f.createErr
	}
	return models.AddResponse{ID: "created-" + req.ModelID, ModelID: req.ModelID}, nil
}

func TestSpeechServiceProviderListGetAndMeta(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC)
	audio := &fakeSpeechAudioService{
		providers: []audiopkg.SpeechProviderResponse{{
			ID:         "provider-1",
			Name:       "OpenAI Speech",
			ClientType: "openai-speech",
			Enable:     true,
			Config:     map[string]any{"api_key": "masked"},
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
		meta: []audiopkg.ProviderMetaResponse{{
			Provider:    "openai-speech",
			DisplayName: "OpenAI Speech",
			ConfigSchema: audiopkg.ConfigSchema{Fields: []audiopkg.FieldSchema{{
				Key:      "api_key",
				Type:     "password",
				Required: true,
			}}},
		}},
	}
	service := &SpeechService{audio: audio}

	listResp, err := service.ListSpeechProviders(context.Background(), connect.NewRequest(&privatev1.ListSpeechProvidersRequest{}))
	if err != nil {
		t.Fatalf("ListSpeechProviders returned error: %v", err)
	}
	if listResp.Msg.GetProviders()[0].GetType() != "openai-speech" {
		t.Fatalf("provider type = %q, want openai-speech", listResp.Msg.GetProviders()[0].GetType())
	}
	if listResp.Msg.GetProviders()[0].GetConfig().AsMap()["api_key"] != "masked" {
		t.Fatalf("provider config was not mapped")
	}

	getResp, err := service.GetSpeechProvider(context.Background(), connect.NewRequest(&privatev1.GetSpeechProviderRequest{Id: "provider-1"}))
	if err != nil {
		t.Fatalf("GetSpeechProvider returned error: %v", err)
	}
	if getResp.Msg.GetProvider().GetName() != "OpenAI Speech" {
		t.Fatalf("provider name = %q, want OpenAI Speech", getResp.Msg.GetProvider().GetName())
	}

	metaResp, err := service.ListSpeechProviderMeta(context.Background(), connect.NewRequest(&privatev1.ListSpeechProviderMetaRequest{}))
	if err != nil {
		t.Fatalf("ListSpeechProviderMeta returned error: %v", err)
	}
	if metaResp.Msg.GetProviders()[0].GetSchema().AsMap()["fields"] == nil {
		t.Fatal("provider schema fields missing")
	}
}

func TestSpeechServiceImportModelsFiltersAndCreatesSpeechModels(t *testing.T) {
	t.Parallel()

	audio := &fakeSpeechAudioService{
		remoteModels: []audiopkg.ModelInfo{
			{ID: "tts-1", Name: "TTS One"},
			{ID: "tts-2", Name: "TTS Two"},
		},
		createdModels: map[string]audiopkg.SpeechModelResponse{
			"created-tts-2": {
				ID:         "created-tts-2",
				ModelID:    "tts-2",
				Name:       "TTS Two",
				ProviderID: "provider-1",
			},
		},
	}
	modelsAdmin := &fakeSpeechModelAdmin{}
	service := &SpeechService{audio: audio, models: modelsAdmin}

	resp, err := service.ImportSpeechProviderModels(context.Background(), connect.NewRequest(&privatev1.ImportSpeechProviderModelsRequest{
		ProviderId: "provider-1",
		ModelIds:   []string{"tts-2"},
	}))
	if err != nil {
		t.Fatalf("ImportSpeechProviderModels returned error: %v", err)
	}
	if len(modelsAdmin.createCalls) != 1 {
		t.Fatalf("create calls = %d, want 1", len(modelsAdmin.createCalls))
	}
	if modelsAdmin.createCalls[0].Type != models.ModelTypeSpeech {
		t.Fatalf("created type = %q, want speech", modelsAdmin.createCalls[0].Type)
	}
	if resp.Msg.GetModels()[0].GetModelId() != "tts-2" {
		t.Fatalf("imported model_id = %q, want tts-2", resp.Msg.GetModels()[0].GetModelId())
	}
}

func TestSpeechServiceModelMethods(t *testing.T) {
	t.Parallel()

	audio := &fakeSpeechAudioService{
		models: []audiopkg.SpeechModelResponse{{
			ID:         "model-1",
			ModelID:    "tts-1",
			Name:       "TTS One",
			ProviderID: "provider-1",
			Config:     map[string]any{"voice": "alloy"},
		}},
	}
	service := &SpeechService{audio: audio}

	listResp, err := service.ListSpeechModels(context.Background(), connect.NewRequest(&privatev1.ListSpeechModelsRequest{ProviderId: "provider-1"}))
	if err != nil {
		t.Fatalf("ListSpeechModels returned error: %v", err)
	}
	if listResp.Msg.GetModels()[0].GetDisplayName() != "TTS One" {
		t.Fatalf("display_name = %q, want TTS One", listResp.Msg.GetModels()[0].GetDisplayName())
	}

	getResp, err := service.GetSpeechModel(context.Background(), connect.NewRequest(&privatev1.GetSpeechModelRequest{Id: "model-1"}))
	if err != nil {
		t.Fatalf("GetSpeechModel returned error: %v", err)
	}
	if getResp.Msg.GetModel().GetMetadata().AsMap()["voice"] != "alloy" {
		t.Fatal("model metadata was not mapped")
	}

	metadata, err := structpb.NewStruct(map[string]any{"voice": "nova"})
	if err != nil {
		t.Fatalf("new struct: %v", err)
	}
	displayName := "Renamed"
	updateResp, err := service.UpdateSpeechModel(context.Background(), connect.NewRequest(&privatev1.UpdateSpeechModelRequest{
		Id:          "model-1",
		DisplayName: &displayName,
		Metadata:    metadata,
	}))
	if err != nil {
		t.Fatalf("UpdateSpeechModel returned error: %v", err)
	}
	if valueOrEmpty(audio.updateReq.Name) != "Renamed" {
		t.Fatalf("updated name = %q, want Renamed", valueOrEmpty(audio.updateReq.Name))
	}
	if updateResp.Msg.GetModel().GetMetadata().AsMap()["voice"] != "nova" {
		t.Fatal("updated metadata was not mapped")
	}

	capsResp, err := service.GetSpeechModelCapabilities(context.Background(), connect.NewRequest(&privatev1.GetSpeechModelCapabilitiesRequest{Id: "model-1"}))
	if err != nil {
		t.Fatalf("GetSpeechModelCapabilities returned error: %v", err)
	}
	if capsResp.Msg.GetCapabilities().AsMap()["formats"].([]any)[0] != "mp3" {
		t.Fatal("capabilities formats were not mapped")
	}

	testResp, err := service.TestSpeechModel(context.Background(), connect.NewRequest(&privatev1.TestSpeechModelRequest{
		Id:   "model-1",
		Text: " hello ",
	}))
	if err != nil {
		t.Fatalf("TestSpeechModel returned error: %v", err)
	}
	if !testResp.Msg.GetOk() || testResp.Msg.GetMessage() != "audio/mpeg" {
		t.Fatalf("test response = %#v, want ok audio/mpeg", testResp.Msg)
	}
	if audio.synthesizeText != "hello" {
		t.Fatalf("synthesize text = %q, want hello", audio.synthesizeText)
	}

	synthResp, err := service.SynthesizeBotSpeech(context.Background(), connect.NewRequest(&privatev1.SynthesizeBotSpeechRequest{
		ModelId: "model-1",
		Text:    " play ",
	}))
	if err != nil {
		t.Fatalf("SynthesizeBotSpeech returned error: %v", err)
	}
	if string(synthResp.Msg.GetAudio()) != "audio" || synthResp.Msg.GetContentType() != "audio/mpeg" {
		t.Fatalf("synthesize response = %#v, want audio/mpeg audio", synthResp.Msg)
	}
	if audio.synthesizeText != "play" {
		t.Fatalf("synthesize text = %q, want play", audio.synthesizeText)
	}
}

func TestSpeechServiceErrors(t *testing.T) {
	t.Parallel()

	service := &SpeechService{audio: &fakeSpeechAudioService{getModelErr: errors.New("speech model not found")}}
	_, err := service.GetSpeechModel(context.Background(), connect.NewRequest(&privatev1.GetSpeechModelRequest{Id: "missing"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}

	_, err = service.TestSpeechModel(context.Background(), connect.NewRequest(&privatev1.TestSpeechModelRequest{
		Id:   "model-1",
		Text: "",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeInvalidArgument, err)
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
