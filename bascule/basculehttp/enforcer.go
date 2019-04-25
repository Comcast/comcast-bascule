package basculehttp

import (
	"context"
	"net/http"

	"github.com/goph/emperror"

	"github.com/Comcast/comcast-bascule/bascule"
	"github.com/go-kit/kit/log/level"
)

//go:generate stringer -type=NotFoundBehavior

type NotFoundBehavior int

// Behavior on not found
const (
	Forbid NotFoundBehavior = iota
	Allow
)

type enforcer struct {
	notFoundBehavior NotFoundBehavior
	rules            map[bascule.Authorization]bascule.Validators
	getLogger        func(context.Context) bascule.Logger
}

func (e *enforcer) decorate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		logger := e.getLogger(ctx)
		if logger == nil {
			logger = bascule.GetDefaultLoggerFunc(ctx)
		}
		auth, ok := bascule.FromContext(ctx)
		if !ok {
			logger.Log(level.Key(), level.ErrorValue(), bascule.ErrorKey, "no authentication found")
			response.WriteHeader(http.StatusForbidden)
			return
		}
		rules, ok := e.rules[auth.Authorization]
		if !ok {
			logger.Log(level.Key(), level.ErrorValue(),
				bascule.ErrorKey, "no rules found for authorization", "rules", rules,
				"authorization", auth.Authorization, "behavior", e.notFoundBehavior)
			switch e.notFoundBehavior {
			case Forbid:
				response.WriteHeader(http.StatusForbidden)
				return
			case Allow:
				// continue
			default:
				response.WriteHeader(http.StatusForbidden)
				return
			}
		} else {
			err := rules.Check(ctx, auth.Token)
			if err != nil {
				errs := []string{err.Error()}
				if es, ok := err.(bascule.Errors); ok {
					for _, e := range es {
						errs = append(errs, e.Error())
					}
				}
				logger.Log(append(emperror.Context(err), level.Key(), level.ErrorValue(), bascule.ErrorKey, errs)...)
				WriteResponse(response, http.StatusUnauthorized, err)
				return
			}
		}
		logger.Log(level.Key(), level.DebugValue(), "msg", "authentication accepted by enforcer")
		next.ServeHTTP(response, request)
	})
}

type EOption func(*enforcer)

func WithNotFoundBehavior(behavior NotFoundBehavior) EOption {
	return func(e *enforcer) {
		e.notFoundBehavior = behavior
	}
}

func WithRules(key bascule.Authorization, v bascule.Validators) EOption {
	return func(e *enforcer) {
		e.rules[key] = v
	}
}

func WithELogger(getLogger func(context.Context) bascule.Logger) EOption {
	return func(e *enforcer) {
		e.getLogger = getLogger
	}
}

func NewEnforcer(options ...EOption) func(http.Handler) http.Handler {
	e := &enforcer{
		rules:     make(map[bascule.Authorization]bascule.Validators),
		getLogger: bascule.GetDefaultLoggerFunc,
	}

	for _, o := range options {
		o(e)
	}

	return e.decorate
}
