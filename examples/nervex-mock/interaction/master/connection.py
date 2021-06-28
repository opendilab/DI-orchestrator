from abc import ABCMeta, abstractmethod
from functools import wraps
from threading import Lock
from typing import Optional, Any, Mapping, Type, Callable
from uuid import uuid4, UUID

import requests
from requests.exceptions import RequestException

from .base import _BEFORE_HOOK_TYPE, _AFTER_HOOK_TYPE, _ERROR_HOOK_TYPE
from .task import Task, _task_complete, _task_fail
from ..base import random_token, ControllableContext, get_http_engine_class, get_values_from_response
from ..config import DEFAULT_CHANNEL, DEFAULT_SLAVE_PORT

_COMPLETE_TRIGGER_NAME = '__TASK_COMPLETE__'
_FAIL_TRIGGER_NAME = '__TASK_FAIL__'


class _ISlaveConnection(ControllableContext, metaclass=ABCMeta):

    @abstractmethod
    def connect(self):
        raise NotImplementedError  # pragma: no cover

    @abstractmethod
    def disconnect(self):
        raise NotImplementedError  # pragma: no cover

    @abstractmethod
    def new_task(self, data: Optional[Mapping[str, Any]] = None):
        raise NotImplementedError  # pragma: no cover

    def start(self):
        self.connect()

    def close(self):
        self.disconnect()


class SlaveConnection(_ISlaveConnection, metaclass=ABCMeta):

    def __init__(
        self,
        host: str,
        port: Optional[int] = None,
        https: bool = False,
        channel: Optional[int] = None,
        my_address: Optional[str] = None,
        token: Optional[str] = None
    ):
        # meta info part
        self.__channel = channel or DEFAULT_CHANNEL
        self.__my_address = my_address
        self.__token = token or random_token()

        # request part
        self.__http_engine = get_http_engine_class(
            headers={
                'Channel': lambda: str(self.__channel),
                'Token': lambda: self.__token,
            }
        )()(host, port or DEFAULT_SLAVE_PORT, https)

        # threading part
        self.__lock = Lock()
        self.__is_connected = False

        # task part
        self.__tasks = {}

        self.__init_triggers()

    def __request(self, method: str, path: str, data: Optional[Mapping[str, Any]] = None) -> requests.Response:
        return self.__http_engine.request(method, path, data)

    @property
    def is_connected(self) -> bool:
        with self.__lock:
            return self.__is_connected

    def _before_connect(self) -> Mapping[str, Any]:
        pass  # pragma: no cover

    def _after_connect(
            self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str, Any]]
    ) -> Any:
        pass  # pragma: no cover

    def _error_connect(self, error: RequestException) -> Any:
        raise error  # pragma: no cover

    def __connect(self):
        try:
            response = self.__request(
                'POST', '/connect', {
                    'master': {
                        'address': self.__my_address,
                    },
                    'data': (self._before_connect() or {})
                }
            )
        except RequestException as err:
            return self._error_connect(err)
        else:
            self.__is_connected = True
            return self._after_connect(*get_values_from_response(response))

    def connect(self):
        with self.__lock:
            return self.__connect()

    def _before_disconnect(self) -> Mapping[str, Any]:
        pass  # pragma: no cover

    def _after_disconnect(
            self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str, Any]]
    ) -> Any:
        pass  # pragma: no cover

    def _error_disconnect(self, error: RequestException) -> Any:
        raise error  # pragma: no cover

    def __disconnect(self):
        try:
            response = self.__request('DELETE', '/disconnect', {
                'data': self._before_disconnect() or {},
            })
        except RequestException as err:
            print(err)
            return self._error_disconnect(err)
        else:
            self.__is_connected = False
            return self._after_disconnect(*get_values_from_response(response))

    def disconnect(self):
        with self.__lock:
            return self.__disconnect()

    def _after_update_learner(
            self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str, Any]]
    ) -> Any:
        return data

    def _error_update_learners(self, error: RequestException) -> Any:
        raise error

    def __update_learners(self, learners):
        try:
            response = self.__request('POST', '/learners', learners)
        except RequestException as err:
            return self._error_update_learners(err)
        else:
            return self._after_update_learner(*get_values_from_response(response))

    def update_learners(self, learners):
        with self.__lock:
            return self.__update_learners(learners)

    def _before_new_task(self, data: Optional[Mapping[str, Any]] = None) -> Mapping[str, Any]:
        return data  # pragma: no cover

    def _after_new_task(
            self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str, Any]]
    ) -> Any:
        pass  # pragma: no cover

    def _error_new_task(self, error: RequestException) -> Any:
        raise error  # pragma: no cover

    def new_task(self, data: Optional[Mapping[str, Any]] = None) -> Task:
        with self.__lock:
            _uuid = uuid4()
            _task = Task(
                http_engine=self.__http_engine,
                data=data,
                task_id=_uuid,
                before_task_start=self._before_new_task,
                after_task_start=self._after_new_task,
                error_task_start=self._error_new_task,
            )

            self.__tasks[_uuid] = _task
            return _task

    def __task_complete(self, task_id: UUID, task_result: Mapping[str, Any]):
        _task = self.__tasks[task_id]
        _task_complete(_task, task_result)
        del self.__tasks[task_id]

    def __task_fail(self, task_id: UUID, task_result: Mapping[str, Any]):
        _task = self.__tasks[task_id]
        _task_fail(_task, task_result)
        del self.__tasks[task_id]

    def __task_complete_trigger(self, task_id: UUID, task_result: Mapping[str, Any]):
        with self.__lock:
            if task_id in self.__tasks.keys():
                return self.__task_complete(task_id, task_result)
            else:
                raise KeyError("Task {uuid} not found in this connection.".format(uuid=repr(str(task_id))))

    def __task_fail_trigger(self, task_id: UUID, task_result: Mapping[str, Any]):
        with self.__lock:
            if task_id in self.__tasks.keys():
                return self.__task_fail(task_id, task_result)
            else:
                raise KeyError("Task {uuid} not found in this connection.".format(uuid=repr(str(task_id))))

    def __init_triggers(self):
        setattr(self, _COMPLETE_TRIGGER_NAME, self.__task_complete_trigger)
        setattr(self, _FAIL_TRIGGER_NAME, self.__task_fail_trigger)


def _connection_task_complete(connection: SlaveConnection, task_id: UUID, task_result: Mapping[str, Any]):
    return getattr(connection, _COMPLETE_TRIGGER_NAME)(task_id, task_result)


def _connection_task_fail(connection: SlaveConnection, task_id: UUID, task_result: Mapping[str, Any]):
    return getattr(connection, _FAIL_TRIGGER_NAME)(task_id, task_result)


class SlaveConnectionProxy(_ISlaveConnection):

    def __init__(
        self,
        connection: SlaveConnection,
        after_connect: Optional[Callable] = None,
        after_disconnect: Optional[Callable] = None
    ):
        self.__connection = connection
        self.__lock = Lock()
        self.__after_connect = after_connect
        self.__after_disconnect = after_disconnect

        self.__init_triggers()

    @property
    def is_connected(self) -> bool:
        with self.__lock:
            return self.__connection.is_connected

    def connect(self):
        with self.__lock:
            result = self.__connection.connect()
            if self.__after_connect is not None:
                self.__after_connect(connection=self)
            return result

    def disconnect(self):
        with self.__lock:
            result = self.__connection.disconnect()
            if self.__after_disconnect is not None:
                self.__after_disconnect(connection=self)
            return result

    def new_task(self, data: Optional[Mapping[str, Any]] = None):
        with self.__lock:
            return self.__connection.new_task(data)
    
    def update_learners(self, learners):
        with self.__lock:
            return self.__connection.update_learners(learners)

    def __task_complete_trigger(self, task_id: UUID, task_result: Mapping[str, Any]):
        with self.__lock:
            return _connection_task_complete(self.__connection, task_id, task_result)

    def __task_fail_trigger(self, task_id: UUID, task_result: Mapping[str, Any]):
        with self.__lock:
            return _connection_task_fail(self.__connection, task_id, task_result)

    def __init_triggers(self):
        setattr(self, _COMPLETE_TRIGGER_NAME, self.__task_complete_trigger)
        setattr(self, _FAIL_TRIGGER_NAME, self.__task_fail_trigger)


def _proxy_task_complete(proxy: SlaveConnectionProxy, task_id: UUID, task_result: Mapping[str, Any]):
    return getattr(proxy, _COMPLETE_TRIGGER_NAME)(task_id, task_result)


def _proxy_task_fail(proxy: SlaveConnectionProxy, task_id: UUID, task_result: Mapping[str, Any]):
    return getattr(proxy, _FAIL_TRIGGER_NAME)(task_id, task_result)


def _slave_task_complete(connection: _ISlaveConnection, task_id: UUID, task_result: Mapping[str, Any]):
    if isinstance(connection, SlaveConnection):
        return _connection_task_complete(connection, task_id, task_result)
    elif isinstance(connection, SlaveConnectionProxy):
        return _proxy_task_complete(connection, task_id, task_result)
    else:
        raise TypeError(
            "{expect1} or {expect2} expected, but {actual} found.".format(
                expect1=SlaveConnection.__name__,
                expect2=SlaveConnectionProxy.__name__,
                actual=type(connection).__name__,
            )
        )


def _slave_task_fail(connection: _ISlaveConnection, task_id: UUID, task_result: Mapping[str, Any]):
    if isinstance(connection, SlaveConnection):
        return _connection_task_fail(connection, task_id, task_result)
    elif isinstance(connection, SlaveConnectionProxy):
        return _proxy_task_fail(connection, task_id, task_result)
    else:
        raise TypeError(
            "{expect1} or {expect2} expected, but {actual} found.".format(
                expect1=SlaveConnection.__name__,
                expect2=SlaveConnectionProxy.__name__,
                actual=type(connection).__name__,
            )
        )


def _default_wrap(func: Callable) -> Callable:

    @wraps(func)
    def _new_func(*args, **kwargs):
        if func:
            return func(*args, **kwargs)
        else:
            return None

    return _new_func


def _get_connection_class(
        before_new_task: Optional[_BEFORE_HOOK_TYPE] = None,
        after_new_task: Optional[_AFTER_HOOK_TYPE] = None,
        error_new_task: Optional[_ERROR_HOOK_TYPE] = None,
        before_connect: Optional[_BEFORE_HOOK_TYPE] = None,
        after_connect: Optional[_AFTER_HOOK_TYPE] = None,
        error_connect: Optional[_ERROR_HOOK_TYPE] = None,
        before_disconnect: Optional[_BEFORE_HOOK_TYPE] = None,
        after_disconnect: Optional[_AFTER_HOOK_TYPE] = None,
        error_disconnect: Optional[_ERROR_HOOK_TYPE] = None,
) -> Type[SlaveConnection]:

    class _Connection(SlaveConnection):

        def _before_connect(self) -> Mapping[str, Any]:
            return _default_wrap(before_connect)() or {}

        def _after_connect(
                self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str,
                                                                                                                 Any]]
        ) -> Any:
            return _default_wrap(after_connect)(status_code, success, code, message, data)

        def _error_connect(self, error: RequestException) -> Any:
            return _default_wrap(error_connect)(error)

        def _before_disconnect(self) -> Mapping[str, Any]:
            return _default_wrap(before_disconnect)() or {}

        def _after_disconnect(
                self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str,
                                                                                                                 Any]]
        ) -> Any:
            return _default_wrap(after_disconnect)(status_code, success, code, message, data)

        def _error_disconnect(self, error: RequestException) -> Any:
            return _default_wrap(error_disconnect)(error)

        def _before_new_task(self, data: Optional[Mapping[str, Any]] = None) -> Mapping[str, Any]:
            return _default_wrap(before_new_task)(data) or {}

        def _after_new_task(
                self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str,
                                                                                                                 Any]]
        ) -> Any:
            return _default_wrap(after_new_task)(status_code, success, code, message, data)

        def _error_new_task(self, error: RequestException) -> Any:
            return _default_wrap(error_new_task)(error)

    return _Connection

class ServerConnection:

    def __init__(
        self,
        host: str,
        port: Optional[int] = None,
        api_version: str = "v1alpha1",
        https: bool = False,
        namespace: str = None,
        name: str = None,
    ):
        # request part
        self.__http_engine = get_http_engine_class(headers={})()(host, port, https)
        self.__api_version = api_version
        self.__namespace = namespace
        self.__my_name = name

    @property
    def api_version(self):
        return self.__api_version

    def __prefix_with_api_version(self, path):
        return self.__api_version + path

    def get_replicas(self, name: str = None):
        try:
            if name is None:
                params = {"namespace": self.__namespace, "coordinator": self.__my_name}
            else:
                params = {"namespace": self.__namespace, "name": name}
            response = self.__http_engine.request('GET', self.__prefix_with_api_version('/replicas'), params=params)
        except RequestException as err:
            return self._error_request(err)
        else:
            return self._after_request(*get_values_from_response(response))
    
    def post_replicas(self, data):
        try:
            data.update(
                {
                    "namespace": self.__namespace, 
                    "coordinator": self.__my_name
                    }
                )
            response = self.__http_engine.request('POST', self.__prefix_with_api_version('/replicas'), data=data)
        except RequestException as err:
            return self._error_request(err)
        else:
            return self._after_request(*get_values_from_response(response))

    def post_replicas_failed(self, collectors=[], learners=[]):
        try:
            data = {
                    "namespace": self.__namespace, 
                    "coordinator": self.__my_name,
                    "collectors": collectors,
                    "learners": learners,
            }
            response = self.__http_engine.request('POST', self.__prefix_with_api_version('/replicas/failed'), data=data)
        except RequestException as err:
            return self._error_request(err)
        else:
            return self._after_request(*get_values_from_response(response))

    def delete_replicas(self, n_collectors=0, n_learners=0):
        try:
            data = {
                "namespace": self.__namespace, 
                "coordinator": self.__my_name,
                "collectors": {
                    "replicas": n_collectors, 
                },
                "learners":{
                    "replicas": n_learners,
                }
            }
            response = self.__http_engine.request('DELETE', self.__prefix_with_api_version('/replicas'), data=data)
        except RequestException as err:
            return self._error_request(err)
        else:
            return self._after_request(*get_values_from_response(response))

    def _after_request(
            self, status_code: int, success: bool, code: int, message: Optional[str], data: Optional[Mapping[str, Any]]
    ) -> Any:
        return success, code, message, data
    
    def _error_request(self, error: RequestException) -> Any:
        # raise error
        pass