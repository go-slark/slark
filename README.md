# slark

base framework lib

框架底层依赖：grpc + gin

TODO list:

3.配置中心特性

5.benchmark

6.配置热更新 --> 用于配置中心？
    【1】在加载配置文件之后，启动一个线程。
    
    【2】该线程定时监听这个配置文件是否有改动。
    
    【3】如果配置文件有变动，就重新加载一下。
    
    【4】重新加载之后通知需要使用这些配置的应用程序（进程或线程），实际上就是刷新内存中配置。

7.服务治理

8.copygen

全面拥抱k8s，暂不支持自定义注册中心

newCtx = context.WithValue(ctx.Request.Context(), header[0], header[1])
header[0], header[1]对应x-token key, value传递