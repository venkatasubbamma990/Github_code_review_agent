package server

import (
	"github.com/gin-gonic/gin"

	"codereviewagent/internal/handler"
	"codereviewagent/internal/middleware"
)

type Server struct {
	engine  *gin.Engine
	handler *handler.ReviewHandler
}

func New(h *handler.ReviewHandler, ginMode string) *Server {
	gin.SetMode(ginMode)
	engine := gin.New()
	engine.Use(middleware.Recovery(), middleware.RequestLogger())

	s := &Server{engine: engine, handler: h}
	s.registerRoutes()
	return s
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) registerRoutes() {
	s.engine.GET("/health", s.handler.Health)

	api := s.engine.Group("/api/v1")
	{
		api.POST("/review", s.handler.ReviewCode)
		api.POST("/review/pr", s.handler.ReviewPR)
		api.POST("/webhooks/github", s.handler.GitHubWebhook)
	}
}
