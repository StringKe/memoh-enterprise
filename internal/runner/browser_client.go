package runner

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	browserv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/browser/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/browser/v1/browserv1connect"
)

const (
	BrowserActionNavigate = "navigate"
	BrowserActionClick    = "click"
	BrowserActionTypeText = "type_text"
	BrowserActionEvaluate = "evaluate"
)

type BrowserClient struct {
	client browserv1connect.BrowserServiceClient
}

type BrowserAction struct {
	Kind       string
	SessionID  string
	URL        string
	Selector   string
	Text       string
	Expression string
	Args       *structpb.Struct
	TimeoutMS  int32
}

type BrowserActionResult struct {
	Status  string
	Session *browserv1.BrowserSession
	Result  *structpb.Value
}

func NewBrowserClient(client browserv1connect.BrowserServiceClient) *BrowserClient {
	return &BrowserClient{client: client}
}

func (c *BrowserClient) CreateContext(ctx context.Context, coreID, deviceID string, options *structpb.Struct) (*browserv1.BrowserContext, error) {
	if c == nil || c.client == nil {
		return nil, ErrBrowserClientMissing
	}
	resp, err := c.client.CreateContext(ctx, connect.NewRequest(&browserv1.CreateContextRequest{
		CoreId:   coreID,
		DeviceId: deviceID,
		Options:  options,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetContext(), nil
}

func (c *BrowserClient) CloseContext(ctx context.Context, contextID string) error {
	if c == nil || c.client == nil {
		return ErrBrowserClientMissing
	}
	_, err := c.client.CloseContext(ctx, connect.NewRequest(&browserv1.CloseContextRequest{ContextId: contextID}))
	return err
}

func (c *BrowserClient) CreateSession(ctx context.Context, contextID, initialURL string) (*browserv1.BrowserSession, error) {
	if c == nil || c.client == nil {
		return nil, ErrBrowserClientMissing
	}
	resp, err := c.client.CreateSession(ctx, connect.NewRequest(&browserv1.CreateSessionRequest{
		ContextId:  contextID,
		InitialUrl: initialURL,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetSession(), nil
}

func (c *BrowserClient) CloseSession(ctx context.Context, sessionID string) error {
	if c == nil || c.client == nil {
		return ErrBrowserClientMissing
	}
	_, err := c.client.CloseSession(ctx, connect.NewRequest(&browserv1.CloseSessionRequest{SessionId: sessionID}))
	return err
}

func (c *BrowserClient) RunAction(ctx context.Context, action BrowserAction) (BrowserActionResult, error) {
	if c == nil || c.client == nil {
		return BrowserActionResult{}, ErrBrowserClientMissing
	}
	switch action.Kind {
	case BrowserActionNavigate:
		resp, err := c.client.Navigate(ctx, connect.NewRequest(&browserv1.NavigateRequest{
			SessionId: action.SessionID,
			Url:       action.URL,
			TimeoutMs: action.TimeoutMS,
		}))
		if err != nil {
			return BrowserActionResult{}, err
		}
		return BrowserActionResult{Status: resp.Msg.GetStatus(), Session: resp.Msg.GetSession()}, nil
	case BrowserActionClick:
		resp, err := c.client.Click(ctx, connect.NewRequest(&browserv1.ClickRequest{
			SessionId: action.SessionID,
			Selector:  action.Selector,
			TimeoutMs: action.TimeoutMS,
		}))
		if err != nil {
			return BrowserActionResult{}, err
		}
		return BrowserActionResult{Status: resp.Msg.GetStatus()}, nil
	case BrowserActionTypeText:
		resp, err := c.client.TypeText(ctx, connect.NewRequest(&browserv1.TypeTextRequest{
			SessionId: action.SessionID,
			Selector:  action.Selector,
			Text:      action.Text,
			TimeoutMs: action.TimeoutMS,
		}))
		if err != nil {
			return BrowserActionResult{}, err
		}
		return BrowserActionResult{Status: resp.Msg.GetStatus()}, nil
	case BrowserActionEvaluate:
		resp, err := c.client.Evaluate(ctx, connect.NewRequest(&browserv1.EvaluateRequest{
			SessionId:  action.SessionID,
			Expression: action.Expression,
			Args:       action.Args,
		}))
		if err != nil {
			return BrowserActionResult{}, err
		}
		return BrowserActionResult{Result: resp.Msg.GetResult()}, nil
	default:
		return BrowserActionResult{}, connect.NewError(connect.CodeInvalidArgument, ErrInvalidRunLease)
	}
}

func (c *BrowserClient) Screenshot(ctx context.Context, sessionID, selector string, fullPage bool, maxBytes int32) (*browserv1.ScreenshotResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrBrowserClientMissing
	}
	resp, err := c.client.Screenshot(ctx, connect.NewRequest(&browserv1.ScreenshotRequest{
		SessionId: sessionID,
		Selector:  selector,
		FullPage:  fullPage,
		MaxBytes:  maxBytes,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
