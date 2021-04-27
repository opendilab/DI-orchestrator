import traceback
import time
import os
import sys
import requests
import json

from queue import Queue, Empty
from typing import Dict, Callable
from threading import Thread

from utils import LockContext, LockContextType
from interaction.master import Master
from interaction.master.task import TaskStatus
from .resource_manager import NaiveResourceManager
from interaction.base import get_http_engine_class, split_http_address

DEFAULT_NAMESPACE = 'default'
DEFAULT_POD_NAME = 'nervexjob-example-coordinator'

init_replicas_request = {
    "actors": {
        "cpu":      "0.1",
        "memory":   "50Mi",
        "replicas": 2,
    },
    "learners": {
        "cpu":      "0.1",
        "memory":   "50Mi",
        "gpu":      "0",
        "replicas": 1,
    },
}

class CoordinatorInteraction(object):

    def __init__(self, cfg: dict, system_addr, callback_fn: Dict[str, Callable], logger: 'TextLogger') -> None:  # noqa
        self._cfg = cfg
        self.system_addr = system_addr
        self._callback_fn = callback_fn
        # self._logger = logger
        self._connection_lock = LockContext(LockContextType.THREAD_LOCK)
        self._connection_actor = {}
        self._connection_learner = {}
        self._connection_agg = None
        server_host, server_port, _, _ = split_http_address(system_addr)
        self.__server_http_engine = get_http_engine_class(headers={})()(server_host, server_port, False)

        self._resource_lock = LockContext(LockContextType.THREAD_LOCK)
        self._resource_manager = NaiveResourceManager()
        self._end_flag = True
        self._remain_lock = LockContext(LockContextType.THREAD_LOCK)
        self._remain_actor_task = set()
        self._remain_learner_task = set()

        self._remain_actor_conn = set()
        self._remain_learner_conn = set()

        self._task_lock = LockContext(LockContextType.THREAD_LOCK)

        self.replicas_update_queue = Queue(65536)
        self._replicas_update_process_thread = Thread(target=self._replicas_update_process)

        self._actor_failed_time = {}


    def _execute_actor_connection(self, _id, host, port):
        print("try to connect to {}:{}".format(host, port))
        max_retry_time = 120
        start_time = time.time()
        while time.time() - start_time <= max_retry_time and not self._end_flag:
            try:
                conn = self._master.new_connection(_id, host, port)
                conn.connect()
                assert conn.is_connected
                resource_task = self._get_resource(conn)
                if resource_task.status != TaskStatus.COMPLETED:
                    # self._logger.error("can't acquire resource for actor({})".format(_id))
                    print("can't acquire resource for actor({})".format(_id))
                    continue
                else:
                    with self._resource_lock:
                        self._resource_manager.update('actor', _id, resource_task.result)
                    with self._connection_lock:
                        self._connection_actor[_id] = conn
                        self._callback_fn['deal_with_increase_actor']()
                    break
            except:
                time.sleep(1)

        with self._remain_lock:
            if _id in self._remain_actor_conn:
                self._remain_actor_conn.remove(_id)
        
        if _id in self._connection_actor and self._connection_actor[_id].is_connected:
            print("Successed to connect to {}:{}".format(host, port))
        else:
            print("Failed to connect to {}:{}".format(host, port))

    # def _execute_learner_connection(self, _id, host, port):
    #     print("try to connect to {}:{}".format(host, port))
    #     max_retry_time = 120
    #     start_time = time.time()
    #     while time.time() - start_time <= max_retry_time and not self._end_flag:
    #         try:
    #             conn = self._master.new_connection(_id, host, port)
    #             conn.connect()
    #             assert conn.is_connected
    #             resource_task = self._get_resource(conn)
    #             if resource_task.status != TaskStatus.COMPLETED:
    #                 # self._logger.error("can't acquire resource for learner({})".format(_id))
    #                 continue
    #             else:
    #                 with self._resource_lock:
    #                     self._resource_manager.update('learner', _id, resource_task.result)
    #                 with self._connection_lock:
    #                     self._connection_learner[_id] = conn
    #                     self._callback_fn['deal_with_increase_learner']()
    #                 break
    #         except:
    #             continue
    #     with self._remain_lock:
    #         if _id in self._remain_learner_conn:
    #             self._remain_learner_conn.remove(_id)
        
    #     if conn.is_connected:
    #         print("Successed to connect to {}:{}".format(host, port))
    #     else:
    #         print("Failed to connect to {}:{}".format(host, port))

    def _execute_aggregator_connection(self, _id, host, port):
        print("try to connect to {}:{}".format(host, port))
        max_retry_time = 120
        start_time = time.time()
        while time.time() - start_time <= max_retry_time and not self._end_flag:
            try:
                conn = self._master.new_connection(_id, host, port)
                conn.connect()
                assert conn.is_connected
                resource_task = self._get_resource(conn)
                if resource_task.status != TaskStatus.COMPLETED:
                    # self._logger.error("can't acquire resource for learner({})".format(_id))
                    print("can't acquire resource for learner({})".format(_id))
                    continue
                else:
                    with self._resource_lock:
                        self._resource_manager.update('learner', _id, resource_task.result)
                    with self._connection_lock:
                        self._connection_learner[_id] = conn
                        self._connection_agg = conn
                        self._callback_fn['deal_with_increase_learner']()
                    break
            except:
                continue
        with self._remain_lock:
            if _id in self._remain_learner_conn:
                self._remain_learner_conn.remove(_id)
        
        if self._connection_agg is not None and self._connection_agg.is_connected:
            print("Successed to connect to {}:{}".format(host, port))
        else:
            print("Failed to connect to {}:{}".format(host, port))

    def _replicas_update_process(self):
        # clear the queue
        while not self.replicas_update_queue.empty():
            self.replicas_update_queue.get()
            
        while not self.replicas_update_queue.empty() or not self._end_flag:
            try:
                _result = self.replicas_update_queue.get(timeout=3.0)
            except Empty:
                continue
            else:
                """
                e.g. _result looks like
                    _result = {
                        'method': 'add',
                        'data': {
                            "namespace": "default",
                            "coordinator": "nervexjob-example-coordinator",
                            "actors": ["localhost:13340"],
                            "learners": ["localhost:12333"],
                        },
                    }
                """
                print("recevied {}".format(_result))
                # Get actors, learners informations from data
                method = _result['method']
                data = _result['data']
                actors, learners = data['actors'], data['learners']

                if method == 'add':
                    if self._connection_agg is None:
                        print('Cannot find aggregator!')
                    else:
                        conn = self._connection_agg
                        conn.added_replicas(learners)

                    # connect to each actor
                    for actor in actors:
                        # actor_id = actor.split(':')[0]
                        actor_id = actor
                        actor_host = actor.split(':')[0]
                        actor_port = actor.split(':')[1]
                        thread = Thread(target=self._execute_actor_connection, args=(actor_id, actor_host, int(actor_port),))
                        thread.start()
                        with self._remain_lock:
                            self._remain_actor_conn.add(actor_id)

                elif method == 'delete':
                    # For simple, here is sequence
                    for actor in actors:
                        actor_id = actor
                        if actor_id not in self._connection_actor:
                            continue
                        
                        # delete the actor from resource manager to ensure that not assign job to actor again
                        while True:
                            with self._resource_lock:
                                if not self._resource_manager.have_assigned('actor', actor_id):
                                    self._resource_manager.delete('actor', actor_id)
                                    break
                            time.sleep(1)

                        with self._connection_lock:
                            conn = self._connection_actor.pop(actor_id)
                        conn.disconnect()
                        self._callback_fn['deal_with_decrease_actor']()
                        assert not conn.is_connected
                        time.sleep(2)

                    if self._connection_agg is None:
                        print('Cannot find aggregator!')
                    else:
                        conn = self._connection_agg
                        conn.deleted_replicas(learners)
                else:
                    raise NotImplementedError

    def send_replicas_request_to_server(self, method, data):
        namespace = os.environ.get('KUBERNETES_POD_NAMESPACE', DEFAULT_NAMESPACE)
        name = os.environ.get('KUBERNETES_POD_NAME', DEFAULT_POD_NAME)

        data["namespace"] = namespace
        data["coordinator"] = name

        response = self.__server_http_engine.request('POST', '/'+method, data=data)
        
        if response.status_code != requests.codes.ok:
            print("Failed to send replicas request to server!")
            sys.exit(1)

    def start(self) -> None:
        self._end_flag = False
        self._replicas_update_process_thread.start()
        self._master = Master(self._cfg.host, self._cfg.port)
        setattr(self._master, 'replicas_update_queue', self.replicas_update_queue)

        self._master.start()
        self._master.ping()

        # Make sure connect to aggregator
        agg_url = os.environ.get('KUBERNETES_AGGREGATOR_URL', "localhost:12334")
        agg_id = agg_url
        agg_host = agg_url.split(":")[0]
        agg_port = agg_url.split(":")[1]
        thread = Thread(target=self._execute_aggregator_connection, args=(agg_id, agg_host, int(agg_port),))
        thread.start()
        max_retry_time = 120
        start_time = time.time()
        while time.time() - start_time <= max_retry_time:
            if self._connection_agg is None:
                time.sleep(2)
            else:
                print("have connected to aggregator")
                break
        if self._connection_agg is None:
            print("can't connect to aggregator, exit!")
            sys.exit(1)

        # send replicas request ot server, then receive the response from /addReplicas http call
        self.send_replicas_request_to_server('addReplicas', init_replicas_request)

        max_retry_time = 120
        start_time = time.time()
        while time.time() - start_time <= max_retry_time:
            if len(self._connection_actor) < self._cfg.actor_limits or len(self._connection_learner) < self._cfg.learner_limits:
                print("Only can connect {} actors, {} learners.".format(len(self._connection_actor), len(self._connection_learner)))
                time.sleep(2)
            else:
                print("Have connected {} actors, {} learners, match limit requests.".format(len(self._connection_actor), len(self._connection_learner)))
                print("Start...")
                break

        if len(self._connection_actor) < self._cfg.actor_limits or len(self._connection_learner) < self._cfg.learner_limits:
            print("Exit since only can connect {} actors, {} learners.".format(len(self._connection_actor), len(self._connection_learner)))
            self.close()
            sys.exit(1)

    def close(self) -> None:
        if self._end_flag:
            return
        self._end_flag = True
        # wait for execute thread
        start_time = time.time()
        while time.time() - start_time <= 60:
            if len(self._remain_learner_task) == 0 and len(self._remain_actor_task) == 0 \
                and len(self._remain_learner_conn) == 0 and len(self._remain_actor_conn) == 0:
                break
            else:
                time.sleep(1)
        for actor_id, conn in self._connection_actor.items():
            conn.disconnect()
            assert not conn.is_connected
        for learner_id, conn in self._connection_learner.items():
            conn.disconnect()
            assert not conn.is_connected
        self._connection_actor = {}
        self._connection_learner = {}
        self._replicas_update_process_thread.join()
        # wait from all slave receive DELETE
        time.sleep(5)
        self._master.close()

    def __del__(self) -> None:
        self.close()

    def _get_resource(self, conn: 'Connection') -> 'TaskResult':  # noqa
        resource_task = conn.new_task({'name': 'resource'})
        resource_task.start().join()
        return resource_task

    def send_actor_task(self, actor_task: dict) -> bool:
        # assert not self._end_flag, "please start interaction first"
        task_id = actor_task['task_id']
        # according to resource info, assign task to a specific actor and adapt task
        with self._resource_lock:
            assigned_actor = self._resource_manager.assign_actor(actor_task)
        if assigned_actor is None:
            # self._logger.error("actor task({}) doesn't have enough actor to execute".format(task_id))
            return False
        actor_task.update(assigned_actor)

        actor_id = actor_task['actor_id']
        start_task = self._connection_actor[actor_id].new_task(
            {
                'name': 'actor_start_task', 
                'task_info': actor_task
            }
        )
        start_task.start().join()
        if start_task.status != TaskStatus.COMPLETED:
            self._resource_manager.update(
                'actor', assigned_actor['actor_id'], assigned_actor['resource_info']
            )
            print('actor_task({}) start failed: {}'.format(task_id, start_task.result))
            self._actor_failed_time[actor_id] = self._actor_failed_time.get(actor_id, 0) + 1
            if self._actor_failed_time[actor_id] >= 5:
                print("ACTOR {} has failed 5 times".format(actor_id))
                self._resource_manager.delete('actor', actor_id)
                self._connection_actor.pop(actor_id)
            return False
        else:
            self._actor_failed_time[actor_id] = self._actor_failed_time.get(actor_id, 0)
            # self._logger.info('actor task({}) is assigned to actor({})'.format(task_id, actor_id))
            print(('actor task({}) is assigned to actor({})'.format(task_id, actor_id)))
            with self._remain_lock:
                self._remain_actor_task.add(task_id)
            actor_task_thread = Thread(target=self._execute_actor_task, args=(actor_task, ))
            actor_task_thread.start()
            return True

    def _execute_actor_task(self, actor_task: dict) -> None:
        actor_id = actor_task['actor_id']
        while not self._end_flag:
            try:
                data_task = self._connection_actor[actor_id].new_task({'name': 'actor_data_task'})
                data_task.start().join()
                if data_task.status != TaskStatus.COMPLETED:
                    # ignore and retry
                    continue
                else:
                    result = data_task.result
                    finished_task = result.pop('finished_task', None)
                    if finished_task:
                        # result['finished_task'] is a flag
                        task_id = result.get('task_id', None)
                        self._callback_fn['deal_with_actor_finish_task'](task_id, result)
                        resource_task = self._get_resource(self._connection_actor[actor_id])
                        if resource_task.status == TaskStatus.COMPLETED:
                            with self._resource_lock:
                                self._resource_manager.update('actor', actor_id, resource_task.result)
                        break
                    else:
                        task_id = result.get('task_id', None)
                        buffer_id = result.get('buffer_id', None)
                        data_id = result.get('data_id', None)
                        self._callback_fn['deal_with_actor_send_data'](task_id, buffer_id, data_id, result)
            except requests.exceptions.HTTPError as e:
                if self._end_flag:
                    break
                else:
                    raise e

        with self._remain_lock:
            self._remain_actor_task.remove(task_id)

    def send_learner_task(self, learner_task: dict) -> bool:
        # assert not self._end_flag, "please start interaction first"
        task_id = learner_task['task_id']
        assigned_learner = self._resource_manager.assign_learner(learner_task)
        if assigned_learner is None:
            # self._logger.error("learner task({}) doesn't have enough learner to execute".format(task_id))
            return False
        learner_task.update(assigned_learner)

        learner_id = learner_task['learner_id']
        start_task = self._connection_learner[learner_id].new_task(
            {
                'name': 'learner_start_task',
                'task_info': learner_task
            }
        )
        start_task.start().join()
        if start_task.status != TaskStatus.COMPLETED:
            return False
        else:
            # self._logger.info('learner task({}) is assigned to learner({})'.format(task_id, learner_id))
            with self._remain_lock:
                self._remain_learner_task.add(task_id)
            learner_task_thread = Thread(target=self._execute_learner_task, args=(learner_task, ))
            learner_task_thread.start()
            return True

    def _execute_learner_task(self, learner_task: dict) -> None:
        learner_id = learner_task['learner_id']
        while not self._end_flag:
            try:
                # get data
                get_data_task = self._connection_learner[learner_id].new_task({'name': 'learner_get_data_task'})
                get_data_task.start().join()
                if get_data_task.status != TaskStatus.COMPLETED:
                    continue
                result = get_data_task.result
                task_id, buffer_id, batch_size = result['task_id'], result['buffer_id'], result['batch_size']
                sleep_count = 1
                while True:
                    data = self._callback_fn['deal_with_learner_get_data'](task_id, buffer_id, batch_size)
                    if self._end_flag or data is not None:
                        break
                    else:
                        time.sleep(sleep_count)
                        sleep_count += 2
                if self._end_flag:
                    break

                # learn
                learn_task = self._connection_learner[learner_id].new_task({'name': 'learner_learn_task', 'data': data})
                learn_task.start().join()
                if learn_task.status != TaskStatus.COMPLETED:
                    continue
                result = learn_task.result
                task_id, finished_task = result['task_id'], result['finished_task']
                # finish task and update resource
                if finished_task:
                    # result['finished_task'] is a flag
                    self._callback_fn['deal_with_learner_finish_task'](task_id, result)
                    resource_task = self._get_resource(self._connection_learner[learner_id])
                    if resource_task.status == TaskStatus.COMPLETED:
                        self._resource_manager.update('learner', learner_id, resource_task.result)
                    break
                else:
                    # update info
                    buffer_id, info = result['buffer_id'], result['info']
                    self._callback_fn['deal_with_learner_send_info'](task_id, buffer_id, info)
            except requests.exceptions.HTTPError as e:
                if self._end_flag:
                    break
                else:
                    raise e

        with self._remain_lock:
            self._remain_learner_task.remove(task_id)
