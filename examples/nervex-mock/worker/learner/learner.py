import os
import argparse
import json
import time

import numpy as np

from easydict import EasyDict

from threading import Thread
from queue import Queue
import random

from interaction import Slave, TaskFail

# Cfg
cfg = {
    'batch_size': 1,
}
cfg = EasyDict(cfg)

def get_args():
    parser = argparse.ArgumentParser(description="Run a simple HTTP server")
    parser.add_argument(
        "-l",
        "--listen",
        default="localhost",
        help="Specify the IP address on which the server listens",
    )
    parser.add_argument(
        "-p",
        "--port",
        type=int,
        default=12333,
        help="Specify the port on which the server listens",
    )
    args = parser.parse_args()
    return args

class MockLearner(Slave):

    def __init__(self, host, port):
        self.__host = host
        self.__port = port

        Slave.__init__(self, host, port, 1)

        self._cfg = cfg

        self._sum = 0
        self._last_iter = 0
        self._current_task_info = None
        self._data_result_queue = Queue(maxsize=1)
        self._data_demand_queue = Queue(maxsize=1)
        self._learn_info_queue = Queue(maxsize=1)

        self._world_size = 0

        self._end_flag = False

    def start(self) -> None:
        Slave.start(self)

    def close(self) -> None:
        if self._end_flag:
            return
        if hasattr(self, '_learner_thread'):
            self._learner_thread.join()
        Slave.close(self)

    def __del__(self) -> None:
        self.close()

    def _process_task(self, task):
        task_name = task['name']
        if task_name == 'resource':
            return {'gpu': 1, 'cpu': 20}
        elif task_name == 'learner_start_task':
            self.deal_with_learner_start(task['task_info'])
            return {'message': 'learner task has started'}
        elif task_name == 'learner_get_data_task':
            data_demand = self._data_demand_queue.get()
            ret = {
                'task_id': self._current_task_info['task_id'],
                'buffer_id': self._current_task_info['buffer_id'],
                'batch_size': data_demand
            }
            return ret
        elif task_name == 'learner_learn_task':
            data = task['data']
            self._data_result_queue.put(data)
            learn_info = self._learn_info_queue.get()
            ret = {
                'task_id': self._current_task_info['task_id'],
                'buffer_id': self._current_task_info['buffer_id'],
                'info': learn_info,
            }
            return ret
        else:
            raise TaskFail(result={'message': 'task name error'}, message='illegal learner task <{}>'.format(task_name))

    def deal_with_learner_start(self, task_info):

        self._current_task_info = task_info
        self._learner_thread = Thread(target=self._mock_run_learner, args=(), daemon=True, name='learner_start')
        self._data_loader_thread = Thread(target=self._mock_run_data_loader, args=(), daemon=True, name='data_loader_start')
        self._learner_thread.start()
        self._data_loader_thread.start()

    def _mock_run_learner(self):
        while not self._end_flag:
            data = self._data_result_queue.get()
            data = [d['data'] for d in data]

            self._sum += np.sum(data)
            self._last_iter += 1

            learner_info = {
                'learner_step': str(self._last_iter),
                'sum': str(self._sum),
                'learner_done': False,
            }
            self._learn_info_queue.put(learner_info)
    
    def _mock_run_data_loader(self):
        while not self._end_flag:
            self._data_demand_queue.put(self._cfg.batch_size)

def main(args):
    learner = MockLearner(args.listen, args.port)
    learner.start()

if __name__ == "__main__":
    args = get_args()
    main(args)