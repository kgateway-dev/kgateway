---
title: Submit a request
weight: 920
description: Solo customers can submit new requests and update existing ones by using the Support Portal, email (`support@solo.io`), or Slack. 
---
Solo customers can submit new requests and update existing ones by using the [Support Portal](#support-portal), email (`support@solo.io`), or [Slack](#slack).
## Use the Support Portal {#support-portal}

### Create a Support Portal account

1. Navigate to the [Support Portal](https://support.solo.io) and click **Sign up**.
2. On the next page, enter your full name and email address. You receive an email with instructions for how to create a password.
3. Follow the **Create password** link in that email. Check your Spam folder if you do not see it in your inbox.
4. You **MUST** add your phone number to your newly created profile. Failing to do so prevents your calls from being routed to a Solo Support Engineer when you call the Support Hotline to report Urgent incidents. 

### Submit a ticket

1. Navigate to the [Support Portal](https://support.solo.io). 
2. Log in, and ensure your phone number is added to your profile. This is an important step, as it allows our system to recognize your account when you call the Support Hotline for urgent incidents.
3. Click **Create a ticket**.
4. Fill out the form. Start by entering any email addresses that you want updates to be sent to while Solo Support works on the ticket. Add a subject related to the issue that you are reporting.
5. Add details for the issue in the **description** field. Refer to [Add support information]({{< versioned_link_path fromRoot="/support/support-info/" >}}) to help Solo Support better understand your request and provide timely and accurate assistance. 
6. Enter the priority for this request. To learn more about priority levels and how to assign the right priority level to your ticket, see [Priority levels](#priority-levels). 
7. Optional: Enter a GitHub issue and upload any attachments. 
   {{< notice note >}}
   The ticketing system has an attachment file size limit of 50MB.
   {{< /notice >}}
8. Submit your request. A Solo Support Engineer will be in touch following the [Targeted times for initial response](#response-time).

To update an existing ticket, log in to your account, and click **View pending tickets**. You see a new page where you can select the ticket you want to update and add comments to.



## Priority levels {#priority-levels}

{{% notice note %}}
For the latest list of priority levels and descriptions, see the [Technical Support Policy](https://legal.solo.io/#technical-support-policy).
{{% /notice %}}

|Priority level|Description|
|----------------|--|
|Urgent priority**|A problem that severely impacts your use of the software in a production environment (such as loss of production data or in which your production systems are not functioning). The situation halts your business operations, or your revenue or brand are impacted and no procedural workaround exists.|
|High priority|A problem where the production environment is operational but functionality is severely reduced. The situation is causing a high impact to portions of your business operations, or your revenue or brand are threatened and no procedural workaround exists.|
|Normal priority|A problem that involves partial, non-critical loss of use of the software in a production environment or development environment. For production environments, there is a medium-to-low impact on your business, but your business continues to function, including by using a procedural workaround. For development environments, where the situation is causing your project to no longer continue or migrate into production.|
|Low priority|A general usage question, reporting of a documentation error, or recommendation for a future product enhancement or modification. For production environments, there is low-to-no impact on your business or the performance or functionality of your system. For development environments, there is a medium-to-low impact on your business, but your business continues to function, including by using a procedural workaround.|

## Targeted times for initial response {#response-time}
  
{{% notice note %}}
For the latest list of targeted times for the initial response, see the [Technical Support Policy](https://legal.solo.io/#technical-support-policy).
{{% /notice %}}

When you contact Solo Support, you can choose a priority for your request:

|Priority Level| Standard Support Policy    |Enhanced Support Policy|
|--|----------------------------|--|
|Urgent`**`| 1 hour (24/7/365)          |15 minutes (24/7/365)|
|High|4 Business Hours Local Time |2 hours (24/7/365)|
|Normal|8 Business Hours Local Time|4 Business Hours Local Time|
|Low|24 Business Hours Local Time|12 Business Hours Local Time|

{{% notice warning %}}
`**`For urgent priority issues that impact production, call the [Support Hotline](https://support.solo.io/hc/en-us/articles/25251551340692-Support-Number).
{{% /notice %}} 

{{% notice note %}}
Solo Support reserves the right to adjust the priority you select if it does not align with the priorities documented above. 
{{% /notice %}}

## Join the solo.io Slack community {#slack}

Join the [solo.io Slack community](https://soloio.slack.com) to participate in conversations with other users and ask questions to the Solo team. 

Navigate through the channels to find topics relevant to you. We recommend `#gloo`<!--, `#gloo-enterprise`, `#service-mesh-hub` for specific product conversations-->, and `#general` for general discussion.

To migrate a discussion from Slack into an email-based support ticket, you can use the `/zendesk create_ticket` macro within the channel where the conversation started. Select **Support** as the assignee to ensure your request reaches the Solo Support team. Selecting other options will delay or prevent our team from seeing your ticket. 

If the macro hasn't been added to a channel, ask a Solo team member to assist you.


## Additional resources

Check out the following guides to start troubleshooting your environment and to collect the information to provide in your support request. 

- Downloadable version of our versioned [Technical Support Policy](https://legal.solo.io/#technical-support-policy)
- [Gloo Gateway contribution guidelines]({{% versioned_link_path fromRoot="/contributing/" %}})
- [Gloo Gateway community](https://github.com/solo-io/gloo/tree/main)

<!--
- [Communities of Practice repository on GitHub](https://github.com/solo-io/solo-cop)
- [Gloo Gateway troubleshooting guide](https://docs.solo.io/gloo-gateway/latest/troubleshooting/)
- [Gloo Mesh troubleshooting guide](https://docs.solo.io/gloo-mesh-enterprise/latest/troubleshooting/)
-->



