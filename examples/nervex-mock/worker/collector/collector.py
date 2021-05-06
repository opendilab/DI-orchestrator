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
from utils import LockContext, LockContextType, get_task_uid

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
        default=8000,
        help="Specify the port on which the server listens",
    )
    args = parser.parse_args()
    return args

class MockCollector(Slave):

    def __init__(self, host, port):
        self.__host = host
        self.__port = port

        Slave.__init__(self, host, port, 1)

        self.data_queue = Queue(10)

        self._collector_close_flag = False
        self._eval_flag = False
        self._target = None

        self._end_flag = False


    def _process_task(self, task):
        task_name = task['name']
        if task_name == 'resource':
            return {'gpu': 1, 'cpu': 20}
        elif task_name == 'collector_start_task':
            self._current_task_info = task['task_info']
            self.deal_with_collector_start(self._current_task_info)
            return {'message': 'collector task has started'}
        elif task_name == 'collector_data_task':
            data = self.deal_with_collector_data_task()
            data['buffer_id'] = self._current_task_info['buffer_id']
            data['task_id'] = self._current_task_info['task_id']
            return data
        elif task_name == 'collector_close_task':
            data = self.deal_with_collector_close()
            return data
        else:
            pass

    def deal_with_collector_start(self, task):
        self._target = task['target']
        self._eval_flag = task['collector_cfg']['eval_flag']
        self._collector_thread = Thread(target=self._mock_collector_gen_data, args=(self._target, ), daemon=True, name='collector_start')
        self._collector_thread.start()

    def deal_with_collector_data_task(self):
        while True:
            if not self.data_queue.empty():
                data = self.data_queue.get()
                break
            else:
                time.sleep(0.1)
        return data

    def deal_with_collector_close(self):
        self._collector_close_flag = True
        self._collector_thread.join()
        del self._collector_thread
        finish_info = {
            'eval_flag': self._eval_flag,
            'target': self._target,
            }
        return finish_info

    def _mock_collector_gen_data(self, target):
        st = time.time()
        num = 0
        while True:
            if not self.data_queue.full():
                num = num + 1
                self.data_queue.put({
                    'data': num,
                    'finished_task': (num == target),
                    'eval_flag': self._eval_flag,
                })
                if num == target:
                    break
            time.sleep(1)

def main(args):
    collector = MockCollector(args.listen, args.port)
    collector.start()

if __name__ == "__main__":
    args = get_args()
    main(args)