package dbutil

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

// Logger interface for database logging
type Logger interface {
	Log(ctx context.Context, level LogLevel, msg string, data map[string]interface{})
}

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	LogLevelTrace LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelNone
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelTrace:
		return "TRACE"
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

// DefaultLogger is a simple logger implementation using Go's standard log package
type DefaultLogger struct {
	minLevel LogLevel
}

// NewDefaultLogger creates a new default logger with the specified minimum log level
func NewDefaultLogger(minLevel LogLevel) *DefaultLogger {
	return &DefaultLogger{minLevel: minLevel}
}

// Log implements the Logger interface
func (l *DefaultLogger) Log(ctx context.Context, level LogLevel, msg string, data map[string]interface{}) {
	if level < l.minLevel {
		return
	}

	logMsg := fmt.Sprintf("[%s] %s", level.String(), msg)
	if len(data) > 0 {
		logMsg += fmt.Sprintf(" %+v", data)
	}

	log.Println(logMsg)
}

// LoggingConnection wraps a Connection with logging capabilities
type LoggingConnection[T Querier] struct {
	*Connection[T]
	logger Logger
}

// WithLogging returns a new connection with logging enabled
func (c *Connection[T]) WithLogging(logger Logger) *LoggingConnection[T] {
	return &LoggingConnection[T]{
		Connection: c,
		logger:     logger,
	}
}

// LoggingConfig holds configuration for database logging
type LoggingConfig struct {
	Logger             Logger
	LogLevel           LogLevel
	LogSlowQueries     bool
	SlowQueryThreshold time.Duration
	LogConnections     bool
	LogTransactions    bool
}

// DefaultLoggingConfig returns a sensible default logging configuration
func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Logger:             NewDefaultLogger(LogLevelInfo),
		LogLevel:           LogLevelInfo,
		LogSlowQueries:     true,
		SlowQueryThreshold: 1 * time.Second,
		LogConnections:     true,
		LogTransactions:    true,
	}
}

// NewConnectionWithLogging creates a new connection with logging enabled
func NewConnectionWithLogging[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T, loggingConfig *LoggingConfig) (*LoggingConnection[T], error) {
	return NewConnectionWithConfigAndLogging(ctx, dsn, newQueriesFunc, nil, loggingConfig)
}

// NewConnectionWithConfigAndLogging creates a new connection with both config and logging
func NewConnectionWithConfigAndLogging[T Querier](ctx context.Context, dsn string, newQueriesFunc func(*pgxpool.Pool) T, cfg *Config, loggingConfig *LoggingConfig) (*LoggingConnection[T], error) {
	if loggingConfig == nil {
		loggingConfig = DefaultLoggingConfig()
	}

	// Create base pool
	pool, err := createPoolWithConfig(ctx, dsn, cfg)
	if err != nil {
		return nil, err
	}

	// Configure pgx tracer if logging is enabled
	if loggingConfig.LogConnections || loggingConfig.LogSlowQueries {
		// We need to reconfigure the pool to add tracing
		pool.Close()

		if dsn == "" {
			searchPath := ""
			if cfg != nil && cfg.SearchPath != "" {
				searchPath = cfg.SearchPath
			}
			dsn = getDSNWithSearchPath(searchPath)
		}

		config, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, err
		}

		// Apply config settings
		config.MaxConns = 10
		config.MinConns = 1
		config.MaxConnLifetime = 30 * time.Minute

		if cfg != nil {
			if cfg.MaxConns > 0 {
				config.MaxConns = cfg.MaxConns
			}
			if cfg.MinConns > 0 {
				config.MinConns = cfg.MinConns
			}
			if cfg.MaxConnLifetime > 0 {
				config.MaxConnLifetime = cfg.MaxConnLifetime
			}
			if cfg.OnConnect != nil {
				config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
					return cfg.OnConnect(conn)
				}
			}
			if cfg.OnDisconnect != nil {
				config.BeforeClose = func(conn *pgx.Conn) {
					cfg.OnDisconnect(conn)
				}
			}
		}

		// Configure pgx tracer
		config.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   &pgxLoggerAdapter{logger: loggingConfig.Logger},
			LogLevel: convertLogLevel(loggingConfig.LogLevel),
		}

		pool, err = pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			return nil, err
		}
	}

	conn := &Connection[T]{
		pool:    pool,
		queries: newQueriesFunc(pool),
		metrics: nil,
	}

	return &LoggingConnection[T]{
		Connection: conn,
		logger:     loggingConfig.Logger,
	}, nil
}

// WithTransaction executes the given function within a database transaction with logging
func (lc *LoggingConnection[T]) WithTransaction(ctx context.Context, fn TransactionFunc[T]) error {
	start := time.Now()
	lc.logger.Log(ctx, LogLevelDebug, "transaction started", nil)

	err := lc.Connection.WithTransaction(ctx, fn)
	duration := time.Since(start)

	if err != nil {
		lc.logger.Log(ctx, LogLevelError, "transaction failed", map[string]interface{}{
			"duration": duration,
			"error":    err.Error(),
		})
	} else {
		lc.logger.Log(ctx, LogLevelDebug, "transaction completed", map[string]interface{}{
			"duration": duration,
		})
	}

	return err
}

// pgxLoggerAdapter adapts our Logger interface to pgx's tracelog.Logger interface
type pgxLoggerAdapter struct {
	logger Logger
}

func (a *pgxLoggerAdapter) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
	logLevel := convertFromPgxLogLevel(level)
	a.logger.Log(ctx, logLevel, msg, data)
}

// convertLogLevel converts our LogLevel to pgx's tracelog.LogLevel
func convertLogLevel(level LogLevel) tracelog.LogLevel {
	switch level {
	case LogLevelTrace:
		return tracelog.LogLevelTrace
	case LogLevelDebug:
		return tracelog.LogLevelDebug
	case LogLevelInfo:
		return tracelog.LogLevelInfo
	case LogLevelWarn:
		return tracelog.LogLevelWarn
	case LogLevelError:
		return tracelog.LogLevelError
	case LogLevelNone:
		return tracelog.LogLevelNone
	default:
		return tracelog.LogLevelInfo
	}
}

// convertFromPgxLogLevel converts pgx's tracelog.LogLevel to our LogLevel
func convertFromPgxLogLevel(level tracelog.LogLevel) LogLevel {
	switch level {
	case tracelog.LogLevelTrace:
		return LogLevelTrace
	case tracelog.LogLevelDebug:
		return LogLevelDebug
	case tracelog.LogLevelInfo:
		return LogLevelInfo
	case tracelog.LogLevelWarn:
		return LogLevelWarn
	case tracelog.LogLevelError:
		return LogLevelError
	case tracelog.LogLevelNone:
		return LogLevelNone
	default:
		return LogLevelInfo
	}
}

// QueryLogger wraps query execution with logging
type QueryLogger[T Querier] struct {
	queries T
	logger  Logger
}

// NewQueryLogger creates a new query logger wrapper
func NewQueryLogger[T Querier](queries T, logger Logger) *QueryLogger[T] {
	return &QueryLogger[T]{
		queries: queries,
		logger:  logger,
	}
}

// LogQuery logs a query execution with timing
func (ql *QueryLogger[T]) LogQuery(ctx context.Context, queryName string, fn func() error) error {
	start := time.Now()

	ql.logger.Log(ctx, LogLevelDebug, "executing query", map[string]interface{}{
		"query": queryName,
	})

	err := fn()
	duration := time.Since(start)

	if err != nil {
		ql.logger.Log(ctx, LogLevelError, "query failed", map[string]interface{}{
			"query":    queryName,
			"duration": duration,
			"error":    err.Error(),
		})
	} else {
		logLevel := LogLevelDebug
		if duration > 1*time.Second {
			logLevel = LogLevelWarn
		}

		ql.logger.Log(ctx, logLevel, "query completed", map[string]interface{}{
			"query":    queryName,
			"duration": duration,
		})
	}

	return err
}

// SlowQueryLogger logs only slow queries
type SlowQueryLogger struct {
	logger    Logger
	threshold time.Duration
}

// NewSlowQueryLogger creates a new slow query logger
func NewSlowQueryLogger(logger Logger, threshold time.Duration) *SlowQueryLogger {
	return &SlowQueryLogger{
		logger:    logger,
		threshold: threshold,
	}
}

// LogIfSlow logs a query only if it exceeds the threshold
func (sql *SlowQueryLogger) LogIfSlow(ctx context.Context, queryName string, duration time.Duration, err error) {
	if duration < sql.threshold {
		return
	}

	data := map[string]interface{}{
		"query":    queryName,
		"duration": duration,
		"slow":     true,
	}

	if err != nil {
		data["error"] = err.Error()
		sql.logger.Log(ctx, LogLevelError, "slow query failed", data)
	} else {
		sql.logger.Log(ctx, LogLevelWarn, "slow query detected", data)
	}
}
