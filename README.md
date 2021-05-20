# nervex-operator

## developer guide
Refers to [developer-guide](./docs/developer-guide.md)

## user guide

### prerequisites
- a well prepared kubernetes cluster. Follow the [instructions](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) to create a kubernetes cluster, or create a local kubernetes node referring to [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) or [minikube](https://minikube.sigs.k8s.io/docs/start/)
- cert-manager. Installation on kubernetes referenced to [cert-manager docs](https://cert-manager.io/docs/installation/kubernetes/). Or you can install by the following command.
```bash
kubectl create -f ./config/certmanager/cert-manager.yaml
```

### install
Install `nervex-operator` and `nervex-server` with the following command.
```bash
kubectl create -f ./config/nervex-manager.yaml
```

`nervex-operator` and `nervex-server` will be installed in `nervex-system` namespace. 
```bash
$ kubectl get pod -n nervex-system
NAME                               READY   STATUS    RESTARTS   AGE
nervex-operator-57cc65d5c9-5vnvn   1/1     Running   0          59s
nervex-server-7b86ff8df4-jfgmp     1/1     Running   0          59s
```

Install global components of NerveXJob defined in AggregatorConfig:
```bash
kubectl create -f examples/nervex-mock-agconfig.yaml -n nervex-system
```
### submit NerveXJob
```bash
# submit NerveXJob
$ kubectl create -f examples/nervex-mock-nervexjob.yaml
nervexjob.nervex.sensetime.com/nervexjob-example-1 created

# get pod and you will see coordinator and aggregator are created
$ kubectl get pod
NAME                              READY   STATUS    RESTARTS   AGE 
nervexjob-example-1-aggregator    1/1     Running   0          8s  
nervexjob-example-1-coordinator   1/1     Running   0          8s

# few seconds later, you will see collectors and learners created by nervex-server
$ kubectl get pod
NAME                                  READY   STATUS    RESTARTS   AGE
nervexjob-example-1-aggregator        1/1     Running   0          80s
nervexjob-example-1-collector-pm5gv   1/1     Running   0          66s
nervexjob-example-1-coordinator       1/1     Running   0          80s
nervexjob-example-1-learner-rcwmc     1/1     Running   0          66s
nervexjob-example-1-learner-txjks     1/1     Running   0          66s

# get logs
$ kubectl logs nervexjob-example-1-coordinator
* Serving Flask app "interaction.master.master" (lazy loading)
 * Environment: production
   WARNING: This is a development server. Do not use it in a production deployment.
   Use a production WSGI server instead.
 * Debug mode: off
try to connect to nervexjob-example-1-aggregator.default:80
can't acquire resource for learner(nervexjob-example-1-aggregator.default:80)
Successed to connect to nervexjob-example-1-aggregator.default:80
have connected to aggregator
Recevied replicas response from server {'namespace': 'default', 'coordinator': 'nervexjob-example-1-coordinator', 'collectors': ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'], 'learners': ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']}
try to connect to nervexjob-example-1-collector-pm5gv.default:80
try to connect to nervexjob-example-1-collector-dz9jl.default:80
failed list:Only can connect 0 collectors, 1 learners.
 [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Only can connect 0 collectors, 1 learners.
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Only can connect 0 collectors, 1 learners.
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Only can connect 0 collectors, 1 learners.
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Only can connect 0 collectors, 1 learners.
Only can connect 0 collectors, 1 learners.
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
Only can connect 0 collectors, 1 learners.
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
Only can connect 0 collectors, 1 learners.
Successed to connect to nervexjob-example-1-collector-dz9jl.default:80
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Have connected 1 collectors, 1 learners, match limit requests.
Start...
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: learner task(learner_task_PID1UUIDb0059cb6-b13a-11eb-8766-8a7e232739e4_1620615118.4310277) put into queue
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) put into queue
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector_task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) can't find proper buffer_id(buffer_PID1UUIDb005a51c-b13a-11eb-8766-8a7e232739e4_1620615118.431186)
failed list: [] []
currnet list: ['nervexjob-example-1-collector-dz9jl.default:80', 'nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
send delete and received {'namespace': 'default', 'coordinator': 'nervexjob-example-1-coordinator', 'collectors': ['nervexjob-example-1-collector-dz9jl.default:80'], 'learners': []}
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector_task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) can't find proper buffer_id(buffer_PID1UUIDb005a51c-b13a-11eb-8766-8a7e232739e4_1620615118.431186)
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) reput into queue
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector_task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) can't find proper buffer_id(buffer_PID1UUIDb005a51c-b13a-11eb-8766-8a7e232739e4_1620615118.431186)
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: learner task(learner_task_PID1UUIDb0059cb6-b13a-11eb-8766-8a7e232739e4_1620615118.4310277) reput into queue
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: replay_buffer(buffer_PID1UUIDb005a51c-b13a-11eb-8766-8a7e232739e4_1620615118.431186) is created
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: learner_task(learner_task_PID1UUIDb0059cb6-b13a-11eb-8766-8a7e232739e4_1620615118.4310277) is successful to be assigned
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDb006f46c-b13a-11eb-a8f4-8a7e232739e4_1620615118.43981) timeout: [1620615124.456363, 1620615118.439875, 6.016488075256348/5]
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-txjks.default:80', 'nervexjob-example-1-learner-rcwmc.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
failed list: [] []
currnet list: ['nervexjob-example-1-collector-pm5gv.default:80'] ['nervexjob-example-1-learner-rcwmc.default:80', 'nervexjob-example-1-learner-txjks.default:80']
Successed to connect to nervexjob-example-1-collector-pm5gv.default:80
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDbe480962-b13a-11eb-a8f4-8a7e232739e4_1620615142.3544374) put into queue
collector task(collector_task_PID1UUIDbe480962-b13a-11eb-a8f4-8a7e232739e4_1620615142.3544374) is assigned to collector(nervexjob-example-1-collector-pm5gv.default:80)
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector_task(collector_task_PID1UUIDbe480962-b13a-11eb-a8f4-8a7e232739e4_1620615142.3544374) is successful to be assigned
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDbe480962-b13a-11eb-a8f4-8a7e232739e4_1620615142.3544374) send data(be49a1aa-b13a-11eb-84d1-5afa89bc32d3)
[Coordinator(PID1UUID9ddfbc06-b13a-11eb-8692-8a7e232739e4_1620615087.9837542)]: collector task(collector_task_PID1UUIDbe480962-b13a-11eb-a8f4-8a7e232739e4_1620615142.3544374) send data(bee26a66-b13a-11eb-84d1-5afa89bc32d3)
```