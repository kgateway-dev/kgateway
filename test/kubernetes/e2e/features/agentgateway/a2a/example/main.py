import logging
import click
from typing import AsyncIterable

from common.server import A2AServer
from common.server.task_manager import InMemoryTaskManager
from common.types import (
    AgentSkill, AgentCapabilities, AgentCard,
    Artifact,
    JSONRPCResponse,
    Message,
    SendTaskRequest,
    SendTaskResponse,
    SendTaskStreamingRequest,
    SendTaskStreamingResponse,
    Task,
    TaskState,
    TaskStatus,
    TaskStatusUpdateEvent,
)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class MyAgentTaskManager(InMemoryTaskManager):
    def __init__(self):
        super().__init__()

    async def on_send_task(self, request: SendTaskRequest) -> SendTaskResponse:
        # Upsert a task stored by InMemoryTaskManager
        await self.upsert_task(request.params)

        task_id = request.params.id
        # Our custom logic that simply marks the task as complete
        # and returns the echo text
        received_text = request.params.message.parts[0].text
        task = await self._update_task(
            task_id=task_id,
            task_state=TaskState.COMPLETED,
            response_text=f"on_send_task received: {received_text}"
        )

        # Send the response
        return SendTaskResponse(id=request.id, result=task)

    async def on_send_task_subscribe(
            self,
            request: SendTaskStreamingRequest
    ) -> AsyncIterable[SendTaskStreamingResponse] | JSONRPCResponse:
        pass

    async def _update_task(
        self,
        task_id: str,
        task_state: TaskState,
        response_text: str,
    ) -> Task:
        task = self.tasks[task_id]
        agent_response_parts = [
            {
                "type": "text",
                "text": response_text,
            }
        ]
        task.status = TaskStatus(
            state=task_state,
            message=Message(
                role="agent",
                parts=agent_response_parts,
            )
        )
        task.artifacts = [
            Artifact(
                parts=agent_response_parts,
            )
        ]
        return task



@click.command()
@click.option("--host", default="0.0.0.0")
@click.option("--port", default=9090)
def main(host, port):
    skill = AgentSkill(
        id="my-project-echo-skill",
        name="Echo Tool",
        description="Echos the input given",
        tags=["echo", "repeater"],
        examples=["I will see this echoed back to me"],
        inputModes=["text"],
        outputModes=["text"],
    )
    logging.info(skill)

    capabilities = AgentCapabilities(streaming=True)
    agent_card = AgentCard(
        name="Echo Agent",
        description="This agent echos the input given",
        url=f"http://{host}:{port}/",
        version="0.1.0",
        defaultInputModes=["text"],
        defaultOutputModes=["text"],
        capabilities=capabilities,
        skills=[skill],
    )
    logging.info(agent_card)

    task_manager = MyAgentTaskManager()
    server = A2AServer(
        agent_card=agent_card,
        task_manager=task_manager,
        host=host,
        port=port,
    )
    server.start()


if __name__ == "__main__":
    main()
