import os
import logging
from pydantic import BaseModel, Field
from langchain.agents import initialize_agent, AgentType
from langchain_openai import ChatOpenAI
from langchain.tools import StructuredTool
from pydantic import SecretStr

TEST_OPENAI_BASE_URL = os.environ.get("TEST_OPENAI_BASE_URL", "")

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class WeatherResponse(BaseModel):
    """Structured weather response for the agent."""

    temperature: float = Field(description="The temperature in Fahrenheit.")
    wind_direction: str = Field(
        description="The direction of the wind in abbreviated form (e.g., 'N', 'SW')."
    )
    wind_speed: float = Field(description="The speed of the wind in km/h.")


# Define a fake weather tool
def fake_get_weather(location: str) -> WeatherResponse:
    """Fake implementation of the weather tool."""
    # Simulated structured response for testing
    return WeatherResponse(
        temperature=72.5,
        wind_direction="NW",
        wind_speed=15.3,
    )


class TestLangchainAgent:
    def test_openai_agent(self):
        # set up the agent
        llm = ChatOpenAI(
            model="gpt-4",
            temperature=0.7,
            base_url=TEST_OPENAI_BASE_URL,
            # langchain openai expects OPENAI_API_KEY to be set or an api key to be passed
            api_key=SecretStr("fake"),
        )
        llm.with_structured_output(WeatherResponse)
        weather_tool = StructuredTool.from_function(fake_get_weather)

        # initialize the agent
        agent_chain = initialize_agent(
            [weather_tool],
            llm,
            agent=AgentType.STRUCTURED_CHAT_ZERO_SHOT_REACT_DESCRIPTION,
            verbose=True,
        )

        # run the agent
        resp = agent_chain.invoke(
            input="What is the weather in Boston?",
        )

        assert resp is not None
        # Validate specific values
        assert "72.5" in resp["output"], "Incorrect temperature."
        assert "15.3" in resp["output"], "Incorrect wind speed."
