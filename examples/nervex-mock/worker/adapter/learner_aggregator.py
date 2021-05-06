from typing import Union, Optional
import traceback
import numbers
import copy
import time
from functools import reduce
from interaction import Master, Slave, TaskFail
from interaction.master.task import TaskStatus
from easydict import EasyDict
from threading import Thread

from utils import LockContext, LockContextType

cfg = {
    'learner': {
        '0': ['learner0', 'localhost', 12333]
    },
}

cfg = EasyDict(cfg)

class LearnerAggregatorSlave(Slave):

    def __init__(self, *args, callback_fn: Optional[dict] = None, **kwargs) -> None:
        print(*args)
        super().__init__(*args, **kwargs)
        self._callback_fn = callback_fn

    def _process_replicas_update(self, method, learners: list):
        if method == 'add':
            self._callback_fn['deal_with_learner_add'](learners)
        elif method == 'delete':
            self._callback_fn['deal_with_learner_delete'](learners)
        return 'OK'

    def _process_task(self, task: dict) -> Union[dict, TaskFail]:
        task_name = task['name']
        if task_name == 'resource':
            return self._callback_fn['deal_with_get_resource']()
        elif task_name == 'learner_start_task':
            return self._callback_fn['deal_with_learner_start'](task)
        elif task_name == 'learner_get_data_task':
            return self._callback_fn['deal_with_get_data'](task)
        elif task_name == 'learner_learn_task':
            return self._callback_fn['deal_with_learn'](task)
        else:
            raise TaskFail(result={'message': 'task name error'}, message='illegal learner task <{}>'.format(task_name))


class MockLearnerAggregator(object):

    def __init__(self, slave_host, slave_port, master_host, master_port) -> None:
        self._cfg = cfg
        callback_fn = {
            'deal_with_get_resource': self.deal_with_get_resource,
            'deal_with_learner_start': self.deal_with_learner_start,
            'deal_with_get_data': self.deal_with_get_data,
            'deal_with_learn': self.deal_with_learn,
            'deal_with_learner_add': self.deal_with_learner_add,
            'deal_with_learner_delete': self.deal_with_learner_delete,

        }

        host, port = slave_host, slave_port
        self._slave = LearnerAggregatorSlave(host, port, callback_fn=callback_fn)
        host, port = master_host, master_port
        self._master = Master(host, port)

        self._connection_lock = LockContext(LockContextType.THREAD_LOCK)
        self._world_size = 0
        self._learner_connection = {}
        self._remain_learner_conn = set()

        self._end_flag = True

    def _execute_learner_connection(self, _id, host, port):
        print("try to connect to {}:{}".format(host, port))
        max_retry_time = 120
        start_time = time.time()
        while time.time() - start_time <= max_retry_time and not self._end_flag:
            try:
                conn = self._master.new_connection(_id, host, port)
                conn.connect()
                assert conn.is_connected
                with self._connection_lock:
                    self._learner_connection[_id] = conn
                    self._world_size += 1
                break
            except:
                continue

        with self._connection_lock:
            if _id in self._remain_learner_conn:
                self._remain_learner_conn.remove(_id)

        if conn.is_connected:
            print("Successed to connect to {}:{}".format(host, port))
        else:
            print("Failed to connect to {}:{}".format(host, port))

    def deal_with_learner_delete(self, learners):
        for learner in learners:
            learner_id = learner.split(':')[0]
            learner_host = learner.split(':')[0]
            learner_port = learner.split(':')[1]

            with self._connection_lock:
                if learner_id in self._learner_connection:
                    conn = self._learner_connection.pop(learner_id)
                    conn.disconnect()
                    assert not conn.is_connected
                    self._world_size -= 1
                else:
                    print("cannot find learner {}".format(learner))

    def deal_with_learner_add(self, learners):
        for learner in learners:
            learner_id = learner.split(':')[0]
            if learner_id in self._learner_connection:
                continue

            learner_host = learner.split(':')[0]
            learner_port = learner.split(':')[1]

            thread = Thread(target=self._execute_learner_connection, args=(learner_id, learner_host, int(learner_port),))
            thread.start()

            with self._connection_lock:
                self._remain_learner_conn.add(learner_id)

    def start(self) -> None:
        self._end_flag = False
        try:
            self._slave.start()
        except Exception as e:
            self._logger.error(
                "learner_aggregator slave start error:\n" + ''.join(traceback.format_tb(e.__traceback__)) + repr(e)
            )
            return

        self._master.start()
        self._master.ping()
        self._world_size = 0

        print("Start...")
        # Since the learner info is assigned by coordinator, aggregator don't need to try to connect learner.
        # max_retry_time = 120
        # start_time = time.time()
        # while time.time() - start_time <= max_retry_time:
        #     if len(self._learner_connection) <= 0:
        #         time.sleep(2)
        #     else:
        #         break

        # # Exceeds max retry time and no learner connection found.
        # if len(self._learner_connection) == 0:
        #     # self._logger.error("learner_aggregator master max retries failed")
        #     print("learner_aggregator master max retries failed")

    def close(self) -> None:
        if self._end_flag:
            return
        self._end_flag = True
        try:
            start_time = time.time()
            while time.time() - start_time <= 60:
                if len(self._remain_learner_conn) == 0:
                    break
                else:
                    time.sleep(1)
            self._slave.close()
            for _, conn in self._learner_connection.items():
                conn.disconnect()
                assert not conn.is_connected
            self._master.close()
        except:  # ignore close exception
            pass

    def deal_with_get_resource(self) -> dict:
        return {'gpu': self._world_size}

    def deal_with_learner_start(self, task: dict) -> dict:
        if len(self._learner_connection) == 0:
            raise TaskFail(message='no connected learner', result={'message': 'no connected learner'})
        name = task['name']
        start_task = {}
        for k, v in self._learner_connection.items():
            start_task[k] = v.new_task({'name': name, 'task_info': task['task_info']})
            start_task[k].start()
        for k, v in start_task.items():
            v.join()
        task_status = [v.status for v in start_task.values()]
        if any([s != TaskStatus.COMPLETED for s in task_status]):
            # TODO(nyz) dynamic learner gpu add/remove
            message = "one of learner can't start_task"
            raise TaskFail(message=message, result={'message': message})
        return {'message': 'learner task has started'}

    def deal_with_get_data(self, task: dict) -> dict:
        data_task = {}
        for k, v in self._learner_connection.items():
            data_task[k] = v.new_task({'name': task['name']})
            data_task[k].start()
        for k, v in data_task.items():
            v.join()
        # TODO deal with task fail
        self._data_demand = {k: v.result for k, v in data_task.items()}
        demand_list = list(self._data_demand.values())
        # Merge data demand info by adding up all learners' demand batch size.
        merged_demand = copy.deepcopy(demand_list[0])
        merged_demand['batch_size'] = sum([d['batch_size'] for d in demand_list])
        return merged_demand

    def deal_with_learn(self, task: dict) -> dict:
        learn_task = {}
        merged_data = task['data']
        # Split training data for each learner according to ``self._data_demand``.
        split_data = []
        start = 0
        for item in self._data_demand.values():
            end = item['batch_size'] + start
            split_data.append(merged_data[start:end])
            start = end
        for (k, v), d in zip(self._learner_connection.items(), split_data):
            learn_task[k] = v.new_task({'name': task['name'], 'data': d})
            learn_task[k].start()
        for k, v in learn_task.items():
            v.join()
        # TODO deal with task fail
        info_list = [v.result for v in learn_task.values()]
        # Merge learn info through ``merge_info`` method.
        merged_info = self.merge_info(info_list)
        return merged_info


    @staticmethod
    def merge_info(info: list) -> dict:
        homogeneous_keys = ['learner_step', 'buffer_id', 'task_id', 'learner_done']
        elem = info[0]
        if elem is None:
            return info
        elif isinstance(elem, numbers.Integral) or isinstance(elem, str) or isinstance(elem, float):
            return info
        elif isinstance(elem, list) or isinstance(elem, tuple):
            return list(reduce(lambda x, y: x + y, info))
        elif isinstance(elem, dict):
            ret = {}
            for k in elem.keys():
                if k in homogeneous_keys:
                    ret[k] = elem[k]
                else:
                    ret[k] = MockLearnerAggregator.merge_info([e[k] for e in info])
            return ret
        else:
            raise TypeError("not support type: {}".format(type(elem)))
