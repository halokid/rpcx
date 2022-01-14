package serverplugin

import (
  "context"
  "fmt"
  "github.com/halokid/ColorfulRabbit"
  "github.com/opentracing/opentracing-go"
  "github.com/opentracing/opentracing-go/ext"
  "github.com/halokid/rpcx-plus/share"
  "github.com/uber/jaeger-client-go"
  "github.com/uber/jaeger-client-go/config"
  "io"
  "log"
  "time"
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
  // todo: share.ReqMetaDataKey is a const varible, just set the key name
  md := ctx.Value(share.ReqMetaDataKey)       // share.ReqMetaDataKey 固定值 "__req_metadata"  可自定义
 //log.Printf("share ReqMetaDataKey ------ %+v\n", share.ReqMetaDataKey)
  log.Printf("GenSpanWhCtx md ------ %+v\n", md)
  var span opentracing.Span

  // todo: get the tracer your declare in your service
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
  log.Printf("GenSpanWhCtx metedata1 --- %+v", metadata)
  // todo: here rewrite the metadata, jaeger will put tracing serialize data in metadata
  err := tracer.Inject(span.Context(), opentracing.TextMap, metadata)
  log.Printf("GenSpanWhCtx metedata2 --- %+v", metadata)
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

func WrapSpan(ctx context.Context, svcOrtag string, operate string, jaeAddr string, timeSpanStart time.Time) {
  go wrapSpanInner(ctx, svcOrtag, operate, jaeAddr, timeSpanStart)
}

func wrapSpanInner(ctx context.Context, svcOrtag string, operate string, jaeAddr string, timeSpanStart time.Time) {
  md := ctx.Value(share.ReqMetaDataKey)
  //log.Printf("add span md ------- %+v", md)
  //now := time.Now()
  var span opentracing.Span
  if md != nil {
    tracer, closer := TraceInit(svcOrtag, jaeAddr)
    //opentracing.SetGlobalTracer(tracer)
    opentracing.InitGlobalTracer(tracer)
    defer closer.Close()
    carrier := opentracing.TextMapCarrier(md.(map[string]string))
    spanContext, err := tracer.Extract(opentracing.TextMap, carrier)
    if err != nil && err != opentracing.ErrSpanContextNotFound {
      log.Printf("wrapSpan metadata error %s\n", err)
    }
    // todo: cost time operation must behind tracer.StartSpan
    span = tracer.StartSpan(svcOrtag, ext.RPCServerOption(spanContext))

    // todo: simuration call java service
    //time.Sleep(800 * time.Millisecond)
    elapsed := time.Since(timeSpanStart)
    log.Printf("wrapSpan elapsed ---- %+v", elapsed)
    time.Sleep(elapsed)

    metadata := opentracing.TextMapCarrier(make(map[string]string))
    log.Printf("wrapSpan %+v metedata1 --- %+v", svcOrtag, metadata)
    // todo: here rewrite the metadata, jaeger will put tracing serialize data in metadata
    err = tracer.Inject(span.Context(), opentracing.TextMap, metadata)
    ColorfulRabbit.CheckError(err, "----- tracer Inject error")
    log.Printf("wrapSpan %+v metedata2 --- %+v", svcOrtag, metadata)
    //span.Finish()
  } else {
    span = opentracing.StartSpan(operate)
    log.Printf("wrapSpan md is nil span ---- %+v", span)
  }
  span.Finish()
}


func GenTracSpan(ctx context.Context, svcOrTag string, subOperate string,
  elapse time.Duration, jaegerAddr string) {
  //svcOrTag := "serv_mysql"
  md := ctx.Value(share.ReqMetaDataKey)
  var span opentracing.Span
  if md != nil {
    tracer, closer := TraceInit(svcOrTag, jaegerAddr)
    //opentracing.SetGlobalTracer(tracer)
    opentracing.InitGlobalTracer(tracer)
    defer closer.Close()
    carrier := opentracing.TextMapCarrier(md.(map[string]string))
    spanContext, err := tracer.Extract(opentracing.TextMap, carrier)
    if err != nil && err != opentracing.ErrSpanContextNotFound {
      log.Printf("wrapSpan metadata error %s\n", err)
    }
    // todo: cost time operation must behind tracer.StartSpan
    span = tracer.StartSpan(subOperate, ext.RPCServerOption(spanContext))

    // todo: simuration call java service
    //time.Sleep(800 * time.Millisecond)
    time.Sleep(elapse)

    metadata := opentracing.TextMapCarrier(make(map[string]string))
    log.Printf("wrapSpan %+v metedata1 --- %+v", svcOrTag, metadata)
    // todo: here rewrite the metadata, jaeger will put tracing serialize data in metadata
    err = tracer.Inject(span.Context(), opentracing.TextMap, metadata)
    ColorfulRabbit.CheckError(err, "----- tracer Inject error")
    log.Printf("wrapSpan %+v metedata2 --- %+v", svcOrTag, metadata)
    //span.Finish()
  } else {
    span = opentracing.StartSpan(subOperate)
    log.Printf("wrapSpan md is nil span ---- %+v", span)
  }
  span.Finish()
}

//func GenBeCallSpanRoot(ctx context.Context, operation string) (io.Closer, opentracing.Span) {
  //operation := "Hris-GetUsers"
  //ctx, closer, spanRoot := GenTracSpanRoot(ctx, operation)
  //return closer, spanRoot
//}


func GenBeCallSpanRoot(ctx context.Context, svcName string,
  operationName string, jaegerAddr string) (context.Context,
  io.Closer, opentracing.Span) {
  tracer, closer := TraceInit(svcName, jaegerAddr)
  opentracing.InitGlobalTracer(tracer)
  //defer closer.Close()

  //tracer := opentracing.GlobalTracer()
  //opentracing.InitGlobalTracer(tracer)

  spanRoot, ctx, err := GenSpanWhCtx(ctx, operationName)
  ColorfulRabbit.CheckError(err, "--- GetUsers tracing span err")
  //defer spanRoot.Finish()
  return ctx, closer, spanRoot
}









