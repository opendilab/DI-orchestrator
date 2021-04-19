import os
import argparse
import json
import time

import traceback
import requests
from typing import Dict, Callable, List
from threading import Thread
from easydict import EasyDict
from queue import Queue
import random

from interaction.master import Master
from interaction.master.task import TaskStatus
from .coordinator_interaction import CoordinatorInteraction
from .base_parallel_commander import create_parallel_commander

from data import ReplayBuffer
from utils import LockContext, LockContextType, get_task_uid


cfg = {
    'interaction': {
        'actor_limits': 1,
        'learner_limits': 1,
        'actor': {
            '0': ['actor0', 'localhost', 13339],
            # '1': ['actor1', 'localhost', '8001'],
        },
        'learner': {
            '0': ['learner0', 'localhost', 12333]
            # '0': ['learner0', 'localhost', 12334]
        },
    },
    'commander': {
        'parallel_commander_type': 'solo',
        'import_names': ['worker.coordinator.solo_parallel_commander'],
        'actor_task_space': 0,
        'learner_task_space': 0,
        'actor_cfg': {
            'env_kwargs': {
                'eval_stop_ptg': 0.2,   
            }
        },
        'learner_cfg': {
            'dataloader': {
                'batch_size': 128,
            }
        },
        'replay_buffer_cfg': {},
        'policy': {},
        'max_iterations': 10000,
        'eval_interval': 5,
    },
    'actor_task_timeout': 5,
    'learner_task_timeout': 5,
}

cfg = EasyDict(cfg)


class TaskState(object):

    def __init__(self, task_id: str) -> None:
        self.task_id = task_id
        self.start_time = time.time()


class MockCoordinator(object):

    def __init__(self, host, port, nervex_server) -> None:
        self.__host = host
        self.__port = port

        cfg.interaction.host = host
        cfg.interaction.port = port

        self.__nerver_server = nervex_server

        self._coordinator_uid = get_task_uid()
        self._cfg = cfg
        self._actor_task_timeout = cfg.actor_task_timeout
        self._learner_task_timeout = cfg.learner_task_timeout

        self._callback = {
            'deal_with_actor_send_data': self.deal_with_actor_send_data,
            'deal_with_actor_finish_task': self.deal_with_actor_finish_task,
            'deal_with_learner_get_data': self.deal_with_learner_get_data,
            'deal_with_learner_send_info': self.deal_with_learner_send_info,
            'deal_with_learner_finish_task': self.deal_with_learner_finish_task,
            'deal_with_increase_actor': self.deal_with_increase_actor,
            'deal_with_decrease_actor': self.deal_with_decrease_actor,
            'deal_with_increase_learner': self.deal_with_increase_learner,
            'deal_with_decrease_learner': self.deal_with_decrease_learner,
        }
        # self._logger, _ = build_logger(path='./log', name='coordinator')
        self._logger = None
        self._interaction = CoordinatorInteraction(cfg.interaction, self.__nerver_server, self._callback, self._logger)
        self._learner_task_queue = Queue()
        self._actor_task_queue = Queue()
        self._commander = create_parallel_commander(cfg.commander)
        self._commander_lock = LockContext(LockContextType.THREAD_LOCK)

        # ############## Thread #####################
        self._assign_actor_thread = Thread(target=self._assign_actor_task, args=())
        self._assign_learner_thread = Thread(target=self._assign_learner_task, args=())
        self._produce_actor_thread = Thread(target=self._produce_actor_task, args=())
        self._produce_learner_thread = Thread(target=self._produce_learner_task, args=())

        self._replay_buffer = {}
        self._task_state = {}  # str -> TaskState
        self._historical_task = []
        # TODO remove used data
        # TODO load/save state_dict

        self._end_flag = True

    def _assign_actor_task(self) -> None:
        r"""
        Overview:
            The function to be called in the assign_actor_task thread.
            Will get an actor task from ``actor_task_queue`` and assign the task.
        """
        while not self._end_flag:
            time.sleep(0.01)
            # get valid task, abandon timeout task
            if self._actor_task_queue.empty():
                continue
            else:
                actor_task, put_time = self._actor_task_queue.get()
                start_retry_time = time.time()
                max_retry_time = 0.3 * self._actor_task_timeout
                while True:
                    # timeout or assigned to actor
                    get_time = time.time()
                    if get_time - put_time >= self._actor_task_timeout:
                        self.info(
                            'actor task({}) timeout: [{}, {}, {}/{}]'.format(
                                actor_task['task_id'], get_time, put_time, get_time - put_time, self._actor_task_timeout
                            )
                        )
                        with self._commander_lock:
                            self._commander.notify_fail_actor_task(actor_task)
                        break
                    buffer_id = actor_task['buffer_id']
                    if buffer_id in self._replay_buffer:
                        if self._interaction.send_actor_task(actor_task):
                            self._record_task(actor_task)
                            self.info("actor_task({}) is successful to be assigned".format(actor_task['task_id']))
                            print("actor_task({}) is successful to be assigned".format(actor_task['task_id']))
                            break
                        else:
                            self.info("actor_task({}) is failed to be assigned".format(actor_task['task_id']))
                    else:
                        self.info(
                            "actor_task({}) can't find proper buffer_id({})".format(actor_task['task_id'], buffer_id)
                        )
                    if time.time() - start_retry_time >= max_retry_time:
                        # reput into queue
                        self._actor_task_queue.put([actor_task, put_time])
                        start_retry_time = time.time()
                        self.info("actor task({}) reput into queue".format(actor_task['task_id']))
                        break
                    time.sleep(3)

    def _assign_learner_task(self) -> None:
        r"""
        Overview:
            The function to be called in the assign_learner_task thread.
            Will take a learner task from learner_task_queue and assign the task.
        """
        while not self._end_flag:
            time.sleep(0.01)
            if self._learner_task_queue.empty():
                continue
            else:
                learner_task, put_time = self._learner_task_queue.get()
                start_retry_time = time.time()
                max_retry_time = 0.1 * self._learner_task_timeout
                while True:
                    # timeout or assigned to learner
                    get_time = time.time()
                    if get_time - put_time >= self._learner_task_timeout:
                        self.info(
                            'learner task({}) timeout: [{}, {}, {}/{}]'.format(
                                learner_task['task_id'], get_time, put_time, get_time - put_time,
                                self._learner_task_timeout
                            )
                        )
                        with self._commander_lock:
                            self._commander.notify_fail_learner_task(learner_task)
                        break
                    if self._interaction.send_learner_task(learner_task):
                        self._record_task(learner_task)
                        # create replay_buffer
                        buffer_id = learner_task['buffer_id']
                        if buffer_id not in self._replay_buffer:
                            replay_buffer_cfg = learner_task.pop('replay_buffer_cfg', {})
                            self._replay_buffer[buffer_id] = ReplayBuffer(replay_buffer_cfg)
                            self._replay_buffer[buffer_id].run()
                            self.info("replay_buffer({}) is created".format(buffer_id))
                        self.info("learner_task({}) is successful to be assigned".format(learner_task['task_id']))
                        break
                    if time.time() - start_retry_time >= max_retry_time:
                        # reput into queue
                        self._learner_task_queue.put([learner_task, put_time])
                        start_retry_time = time.time()
                        self.info("learner task({}) reput into queue".format(learner_task['task_id']))
                        break
                    time.sleep(3)

    def _produce_actor_task(self) -> None:
        r"""
        Overview:
            The function to be called in the ``produce_actor_task`` thread.
            Will ask commander to produce a actor task, then put it into ``actor_task_queue``.
        """
        while not self._end_flag:
            time.sleep(0.01)
            with self._commander_lock:
                actor_task = self._commander.get_actor_task()
                if actor_task is None:
                    continue
            self.info("actor task({}) put into queue".format(actor_task['task_id']))
            self._actor_task_queue.put([actor_task, time.time()])

    def _produce_learner_task(self) -> None:
        r"""
        Overview:
            The function to be called in the produce_learner_task thread.
            Will produce a learner task and put it into the learner_task_queue.
        """
        while not self._end_flag:
            time.sleep(0.01)
            with self._commander_lock:
                learner_task = self._commander.get_learner_task()
                if learner_task is None:
                    continue
            self.info("learner task({}) put into queue".format(learner_task['task_id']))
            self._learner_task_queue.put([learner_task, time.time()])

    def state_dict(self) -> dict:
        return {}

    def load_state_dict(self, state_dict: dict) -> None:
        pass

    def start(self) -> None:
        self._end_flag = False
        self._interaction.start()
        self._produce_actor_thread.start()
        self._assign_actor_thread.start()
        self._produce_learner_thread.start()
        self._assign_learner_thread.start()

    def close(self) -> None:
        if self._end_flag:
            return
        self._end_flag = True
        time.sleep(1)
        self._produce_actor_thread.join()
        self._assign_actor_thread.join()
        self._produce_learner_thread.join()
        self._assign_learner_thread.join()
        self._interaction.close()
        # close replay buffer
        replay_buffer_keys = list(self._replay_buffer.keys())
        for k in replay_buffer_keys:
            v = self._replay_buffer.pop(k)
            v.close()
        self.info('coordinator is closed')

    def __del__(self) -> None:
        self.close()

    def deal_with_actor_send_data(self, task_id: str, buffer_id: str, data_id: str, data: dict) -> None:
        if task_id not in self._task_state:
            self.error('actor task({}) not in self._task_state when send data, throw it'.format(task_id))
            return
        if buffer_id not in self._replay_buffer:
            self.error("actor task({}) data({}) doesn't have proper buffer_id({})".format(task_id, data_id, buffer_id))
            return
        self._replay_buffer[buffer_id].push_data(data)
        self.info('actor task({}) send data({})'.format(task_id, data_id))

    def deal_with_actor_finish_task(self, task_id: str, finished_task: dict) -> None:
        if task_id not in self._task_state:
            self.error('actor task({}) not in self._task_state when finish, throw it'.format(task_id))
            return
        # finish_task
        with self._commander_lock:
            system_shutdown_flag = self._commander.finish_actor_task(task_id, finished_task)
        self._task_state.pop(task_id)
        self._historical_task.append(task_id)
        self.info('actor task({}) is finished'.format(task_id))
        if system_shutdown_flag:
            self.info('coordinator will be closed')
            close_thread = Thread(target=self.close, args=())
            close_thread.start()

    def deal_with_increase_actor(self):
        with self._commander_lock:
            self._commander.increase_actor_task_space()

    def deal_with_decrease_actor(self):
        with self._commander_lock:
            self._commander.decrease_actor_task_space()

    def deal_with_increase_learner(self):
        with self._commander_lock:
            self._commander.increase_learner_task_space()

    def deal_with_decrease_learner(self):
        with self._commander_lock:
            self._commander.decrease_learner_task_space()

    def deal_with_learner_get_data(self, task_id: str, buffer_id: str, batch_size: int) -> List[dict]:
        if task_id not in self._task_state:
            self.error("learner task({}) get data doesn't have proper task_id".format(task_id))
            raise RuntimeError(
                "invalid learner task_id({}) for get data, valid learner_id is {}".format(
                    task_id, self._task_state.keys()
                )
            )
        if buffer_id not in self._replay_buffer:
            self.error("learner task({}) get data doesn't have proper buffer_id({})".format(task_id, buffer_id))
            return
        self.info("learner task({}) get data".format(task_id))
        return self._replay_buffer[buffer_id].sample(batch_size)

    def deal_with_learner_send_info(self, task_id: str, buffer_id: str, info: dict) -> None:
        if task_id not in self._task_state:
            self.error("learner task({}) send info doesn't have proper task_id".format(task_id))
            raise RuntimeError(
                "invalid learner task_id({}) for send info, valid learner_id is {}".format(
                    task_id, self._task_state.keys()
                )
            )
        if buffer_id not in self._replay_buffer:
            self.error("learner task({}) send info doesn't have proper buffer_id({})".format(task_id, buffer_id))
            return
        # self._replay_buffer[buffer_id].update(info['priority_info'])
        with self._commander_lock:
            self._commander.get_learner_info(task_id, info)
        self.info("learner task({}) send info".format(task_id))

    def deal_with_learner_finish_task(self, task_id: str, finished_task: dict) -> None:
        if task_id not in self._task_state:
            self.error("learner task({}) finish task doesn't have proper task_id".format(task_id))
            raise RuntimeError(
                "invalid learner task_id({}) for finish task, valid learner_id is {}".format(
                    task_id, self._task_state.keys()
                )
            )
        with self._commander_lock:
            buffer_id = self._commander.finish_learner_task(task_id, finished_task)
        self._task_state.pop(task_id)
        self._historical_task.append(task_id)
        self.info("learner task({}) finish".format(task_id))
        # delete replay buffer
        if buffer_id is not None:
            replay_buffer = self._replay_buffer.pop(buffer_id)
            replay_buffer.close()
            self.info('replay_buffer({}) is closed'.format(buffer_id))

    def info(self, s: str) -> None:
        # self._logger.info('[Coordinator({})]: {}'.format(self._coordinator_uid, s))
        # print('[Coordinator({})]: {}'.format(self._coordinator_uid, s))
        pass

    def error(self, s: str) -> None:
        # self._logger.error('[Coordinator({})]: {}'.format(self._coordinator_uid, s))
        pass

    def _record_task(self, task: dict):
        self._task_state[task['task_id']] = TaskState(task['task_id'])

    @property
    def system_shutdown_flag(self) -> bool:
        return self._end_flag