# e2e tests
this a separate and unrelated (GO dependencies wise) GO project to tests our Kestra CLI.

it will build a temporary executable binary or kestra cli, and then use go.Command(..) to run real commands
it requires a running Kestra ee instance with this configuration: [docker-setup](docker-setup)

## run tests
either use a running Kestra ee instance and do 
```
# in directory kestra-cli/e2e_tests
go test ./...
```

or 

```
# in directory kestra-cli
sh -c run-e2e-tests.sh
```