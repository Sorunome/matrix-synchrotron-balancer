# matrix-synchrotron-balancer
This is a load balancer for synapse synchrotron workers. As specified in the [docs](https://github.com/matrix-org/synapse/blob/master/docs/workers.rst#synapseappsynchrotron) it is best if each synchrotron handles one user. As such, this load balancer parses that. In addition it also does some basic logic to cycle users to other synchrotrons, if load is too high.

**IMPORTANT** This does not do any authentification at all, so be sure that only `localhost` has access to it!

## Installation
```bash
git clone https://github.com/Sorunome/matrix-synchrotron-balancer
cd matrix-synchrotron-balancer
./install.sh
GOPATH=$(pwd) go build src/matrix-synchrotron-balancer/main.go 
```
## Running
```bash
./main
```
## Configuration
An example file is in `config.sample.yaml`, copy that one to `config.yaml`. Edit it to your needs. Here are all the keys:

 - `homeserver_url`: (string) url of your homeserver
 - `listener`: (string) listener where the load balancer listens to (`host:port`)
 - `synchrotrons`: (array) the defined synchrotrons
   - `address`: (string) address where the synchrotron listens to (`host:port`, WITHOUT `http://`)
   - `pid_file`: (string) the full path of the PID file of the synchrotron
 - `balancer`: Balancer configs
   - `interval`: (int) interval, in seconds, how often the balancer does logic
   - `relocate_min_cpu`: (float) only relocate users if the synchrotron has a CPU usage of at least this much
   - `relocate_threashold`: (float) if the maximum synchrotron load is this much larger than the minimum one, start relocating
   - `relocate_counter_threashold`: (float) to limit sudden bursts to relocate stuff unneededly, relocate only after this many balancer ticks have progressed
   - `relocate_cooldown`: (float) how much the relocate counter is decreased per relocated user
