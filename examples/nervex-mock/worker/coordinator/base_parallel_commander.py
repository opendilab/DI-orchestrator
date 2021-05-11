from abc import ABC, abstractmethod
from collections import defaultdict
from easydict import EasyDict
from utils import import_module


class BaseCommander(ABC):

    @abstractmethod
    def get_collector_task(self) -> dict:
        raise NotImplementedError


class NaiveCommander(BaseCommander):

    def __init__(self, cfg: dict) -> None:
        self._cfg = cfg
        self.collector_task_space = cfg.collector_task_space
        self.learner_task_space = cfg.learner_task_space
        self.collector_task_count = 0
        self.learner_task_count = 0
        self._learner_info = defaultdict(list)
        self._learner_task_finish_count = 0
        self._collector_task_finish_count = 0

    def get_collector_task(self) -> dict:
        if self.collector_task_count < self.collector_task_space:
            self.collector_task_count += 1
            collector_cfg = self._cfg.collector_cfg
            collector_cfg.collect_setting = {'eps': 0.9}
            collector_cfg.eval_flag = False
            return {
                'task_id': 'collector_task_id{}'.format(self.collector_task_count),
                'buffer_id': 'test',
                'collector_cfg': collector_cfg,
                'policy': self._cfg.policy,
            }
        else:
            return None

    def get_learner_task(self) -> dict:
        if self.learner_task_count < self.learner_task_space:
            self.learner_task_count += 1
            learner_cfg = self._cfg.learner_cfg
            learner_cfg.max_iterations = self._cfg.max_iterations
            return {
                'task_id': 'learner_task_id{}'.format(self.learner_task_count),
                'policy_id': 'test.pth',
                'buffer_id': 'test',
                'learner_cfg': learner_cfg,
                'replay_buffer_cfg': self._cfg.replay_buffer_cfg,
                'policy': self._cfg.policy
            }
        else:
            return None

    def finish_collector_task(self, task_id: str, finished_task: dict) -> None:
        self._collector_task_finish_count += 1

    def finish_learner_task(self, task_id: str, finished_task: dict) -> None:
        self._learner_task_finish_count += 1
        return finished_task['buffer_id']

    def notify_fail_collector_task(self, task: dict) -> None:
        pass

    def notify_fail_learner_task(self, task: dict) -> None:
        pass

    def get_learner_info(self, task_id: str, info: dict) -> None:
        self._learner_info[task_id].append(info)


commander_map = {'naive': NaiveCommander}


def register_parallel_commander(name: str, commander: type) -> None:
    assert isinstance(name, str)
    assert issubclass(commander, BaseCommander)
    commander_map[name] = commander


def create_parallel_commander(cfg: dict) -> BaseCommander:
    cfg = EasyDict(cfg)
    import_module(cfg.import_names)
    commander_type = cfg.parallel_commander_type
    if commander_type not in commander_map:
        raise KeyError("not support parallel commander type: {}".format(commander_type))
    else:
        return commander_map[commander_type](cfg)
