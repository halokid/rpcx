package serverplugin

import (
  "context"
  "fmt"
  "github.com/opentracing/opentracing-go"
  "github.com/opentracing/opentracing-go/ext"
  "github.com/halokid/rpcx-plus/share"
  "github.com/uber/jaeger-client-go"
  "github.com/uber/jaeger-client-go/config"
  "io"
  "log"
)


func TraceInit(svc string, atAddr string) (opentracing.Tracer, io.Closer) {
  cfg := &config.Configuration{
    ServiceName: svc,
    Sampler: &config.SamplerConfig{
      Type:  "const",
      Param: 1,
    },
    Reporter: &config.ReporterConfig{
      //LocalAgentHostPort: "127.0.0.1:6831",
      LocalAgentHostPort: atAddr,
      //LogSpans:           true,
      LogSpans:           false,
    },
  }

  tracer, closer, err := cfg.NewTracer(config.Logger(jaeger.StdLogger))
  if err != nil {
    panic(fmt.Sprintf("Init failed: %v\n", err))
  }

  return tracer, closer
}

func GenSpanWhCtx(ctx context.Context, operationName string) (opentracing.Span, context.Context, error) {
  md := ctx.Value(share.ReqMetaDataKey)       // share.ReqMetaDataKey 固定值 "__req_metadata"  可自定义
  var span opentracing.Span

  tracer := opentracing.GlobalTracer()

  if md != nil {
    carrier := opentracing.TextMapCarrier(md.(map[string]string))
    spanContext, err := tracer.Extract(opentracing.TextMap, carrier)
    if err != nil && err != opentracing.ErrSpanContextNotFound {
      log.Printf("metadata error %s\n", err)
      return nil, nil, err
    }
    span = tracer.StartSpan(operationName, ext.RPCServerOption(spanContext))
  } else {
    span = opentracing.StartSpan(operationName)
  }

  metadata := opentracing.TextMapCarrier(make(map[string]string))
  err := tracer.Inject(span.Context(), opentracing.TextMap, metadata)
  if err != nil {
    return nil, nil, err
  }

  //把metdata 携带的 traceid, spanid, parentSpanid 放入 context
  ctx = context.WithValue(context.Background(), share.ReqMetaDataKey, (map[string]string)(metadata))
  return span, ctx, nil
}

func TeSpan(ctx context.Context, atAddr string, svc string, opera string) (opentracing.Span, context.Context, error) {
  // 统一封装开始span
  tracer, closer := TraceInit(svc, atAddr)
  defer closer.Close()
  opentracing.InitGlobalTracer(tracer)

  span, ctxx, err := GenSpanWhCtx(ctx, opera)
  return span, ctxx, err
}


