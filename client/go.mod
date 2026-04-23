module github.com/runner/client

go 1.22

require (
	github.com/joho/godotenv v1.5.1
	github.com/rs/zerolog v1.31.0
	github.com/runner/sdk v0.0.0
	github.com/shirou/gopsutil/v3 v3.23.12
	github.com/spf13/cobra v1.8.0
)

require (
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20231016141302-07b5767bb0ed // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/power-devops/perfstat v0.0.0-20221212215047-62379fc7944b // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.13 // indirect
	github.com/tklauser/numcpus v0.7.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/sys v0.15.0 // indirect
)

replace github.com/runner/sdk => ../sdk/go
