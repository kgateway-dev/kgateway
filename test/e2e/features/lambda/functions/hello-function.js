exports.handler = async (event, context) => {
  // Default to "$LATEST" if no alias is found
  let alias = "$LATEST";
  if (context && context.invokedFunctionArn) {
    const arnParts = context.invokedFunctionArn.split(":");
    alias = arnParts[arnParts.length - 1];
  }

  const response = {
    message: `Hello from Lambda ${alias}`,
    input: event
  };

  return {
    statusCode: 200,
    body: JSON.stringify(response)
  };
};
