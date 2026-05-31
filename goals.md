# Goals

I'm building an app to allow the user to control notifications.

## Audience

Myself initially.

Medium term, other programmers who have the same issues I do.

Long term, anyone who wants to stop being distracted and focus on important work that moves the needle in their life.

## Problems my app solves

* Annoying notifications that are irrelevant to what I'm working on right now
* Fragmented notifications across mobile, desktop etc
* No way of customising notifications in one place
* Lack of granularity in current notification settings - in Github notifications for example, it's impossible to only get notifications on a certain repo for comments only or blocking reviews only
* Confusing and complicated UIs that require me to wade through screens of rules
* Lack of clarity about what notifications will and wont' get through - if I create a rule in these platforms, very often there's no way of me simulating what notifications I'd actually get
* Difficult to change notifications - I've made endless rules and spent hours fiddling with notification rules only for certain notifications to be hidden because I hadn't understood some arcane override rule
* Duplication of notifications - one might appear on my phone, then it also appears on my desktop even though I've already dealt with it
* Big tech has commandeered the notification to get users to interact with their app. They are trading on users attention, so it's not in their interests to help the user really. Instead these tech companies spam us with constant distractions.

## Domain Objects and Concepts

### Signal

**Description** 

An input or a "prod" from the outside world. In other apps these are called notifications.

**Rationale**

At the heart of our app, we're taking a mess of notifications - 99% of which are irrelevant to us - and converting that into actionable notifications that users actually care about. To do that, we need to separate what **might** be relevant to what actually **is** relevant. So a signal is just something that might be relevant to the user. In most cases, the signal will not be relevant.

**Examples**

Email received, WhatsApp message, text message, reminder, calendar appointment upcoming.

**Attributes**

Subject: a summary of the signal. For a direct message that's from an email app, this would be the subject line. For a direct message from a messaging app like WhatsApp, this would be either blank or a summary of the message.

Body: The content of the signal. For an email this would be the email body. For a direct message, this would be the contents of the DM.

App: The app that generated this signal.

Sender: The Contact that sent this signal.

Project: The project that's been assigned. See the rules later for how projects are assigned to signals.

Priority: Important or not. Is the signal worth paying any attention to? Defaults to false. See the rules later for how importance is assigned and remembered.


### Notification

**Description**

An alert telling the user that they need to take an action within a short space of time.

**Rationale**

Most apps abuse notifications, alerting the user on lots of things they don't care about or trying to pull them back into apps to increase their user stats. In our app, a notification has a specific meaning - "Hey, you need to act on this within X minutes".

In Notify, we convert "fake notifications" - things the app builders and big companies care about - into "real actionable notifications" that our users care about.

**Examples**

Your manager just DMed you on Slack. An important meeting is very soon. Your review is needed within the next 2 hours on a PR.

**Attributes**

Subject: copied from the signal

Body: copied from the signal

Maybe other attributes too - not sure.

### Project

**Description**

An ongoing piece of work. See the definition of a project in the Intend.do philosophy below. It could be specific - "Build a house" or vague "Relationship". Projects are always slowly evolving and are personal to the user.

**Rationale**

A way of describing ongoing efforts towards some kind of life improvement. A way of reducing context switching between different mindsets.

**Examples**

Support Mum. Day Job. Build a Business. Parent a Child.

**Attributes**

Name: Name of project.

Colour: Auto assigned, but editable.

Number: Starting at 1.

Shorthand: A calculated field that's the number and the first letter of the project. 1D would be Day Job. 2F might be family. Used when space is at a premium.


## Schedule

**Description**

A recurring time slot where tasks for a project can be completed.

**Rationale**

Schedules can be based on recurring times - "Business Hours: Monday - Friday 9am - 5pm" or "Lunchtime: 12:30pm - 1:20pm". They can also detect holidays in any linked Calendar. So it might be "Monday - Friday 9am to 5pm when I'm in work". "In work" would be defined as "Any day where I don't have a calendar appointment saying "Holiday".

- Schedule - a recurring time slot where tasks for a project can be completed. Schedules can be based on recurring times - "Business Hours: Monday - Friday 9am - 5pm" or "Lunchtime: 12:30pm - 1:20pm". They can also detect holidays in any linked Calendar. So it might be "Monday - Friday 9am to 5pm when I'm in work". "In work" would be defined as "Any day where I don't have a calendar appointment saying "Holiday".

- Sender - the person or organisation sending the signal. If the signal was "PR comment created", the sender would be the author of the comment. If the signal was "Email recieved" the sender would be the sender of the message. If the signal was "Android App JustPark Parking about to expire" the sender would be the organisation - JustPark. Every sender has a unique ID, but that ID could be a phone number, email address, Github username or an app ID, depending on the source of the signal.

- Priority - this is really an attribute, but it's so important I'm breaking it out. Priorities are according to the eisenhower matrix. However, having one sender always be classified as "Urgent and Important" is too simplistic. Instead, classify senders as "Important" or "Unimportant". There could be more complex rules over time.

- Task - created from a signal. A task has the full urgent and important matrix - so four options. A task can sync with other task managers. A task stores a reference to the original signal and the notification associated. By default no signal has a task associated with it. A task belongs to a project.

- App - the app that's generated the notification. An app has a type - Email, Version Control, Messaging. The app also has an name - Facebook, WhatsApp etc. Each app can generate one or more signal types. Example 1: Name: WhatsApp, Type: Messaging, Signal Types: Group Message, Direct Message, Reply. Example 2: Name: Slack, Type: Messaging, Signal Types: Group Message, Direct Message, Reply, Mention. Example 3: Name: Gmail, Type: Email, Signal Types: Group Message, Direct Message, Reply.

- Contact - a person or organisation who has sent an incoming signal. They will have a unique identifier depending on the signal type. So for a WhatsApp notification, it'll be the phone number of the person. For an email, the email address. For a Github notification it'll be the username. For a Slack DM or reply, the unique identifier will be the unique username for that person in that workspace.

## Business Rules

- Projects on creation must have a schedule. They have a name and colour, which is auto assigned. Each project has a number - 1-6 - there can only be 6 projects. Project can have multiple schedules, but these schedules must not overlap, because the schedules are additive.

- Schedules have presets - there's "any time", "waking hours", "lunch", "business hours", "evenings", "weekends". Then the option to create your own.

- Signal to Notification. A signal is turned into a notification if and only if: Signal is important AND Current time and day falls within the schedule of the project assigned.

- Assigning Signals to Projects. The notify app has a signals screen. From the main signals screen, a signal can be assigned to a project. When a signal is assigned to a project that sender or group is assigned to the project. The user can switch up which project has been assigned. They can also set if the project is assigned based on sender or group.  Example - a Whats App message comes in from my brother Mike. This creates a Signal from Mike. This message comes in on the Mums Chat group. I have other groups Mike's part of. So this could be 3 different projects. When I see this signal in the UI for the first time, I must assign the signal to a project. When the signal is assigned to a project, a rule is created that any time that sender creates a signal, it's for that project. However, if the signal comes from a Whats App group (this one does) there's another option to either link the sender and group to the project, or just the group. If a project is assigned based on just the group then when any message comes into that group it'll trigger the notification when the schedule is hit.
  There are three ways of assigning a signal to a project. They each create rules behind the scenes. These three ways, in order of specificity are:
  1. By group and contact - the contact AND group will be associated with the project. Every time a signal comes from that group and that contact, the signal will be assigned to the project.
  2. By contact - the specific contact will be associated with the project. Every time a signal comes from that contact, unless there's an overriding rule, the signal will be assigned to that project.
  3. By group - the group will be associated with the project. Every time a signal comes from that group, no matter who's sent it, the signal will be assigned to that group.
  Overriding rules - the rules will be evaluated in order of specificity. The most specific matching rule would be contact AND group. If a signal matches this rule, all other rules will be ignored. The next most specific is by contact. Then finally by group. Normally the behaviour of rules like this would be difficult to predict. But in this case, we'll have a UI that previews how the rules will be assigned to projects.
  
  - Assigning Contacts to Projects. When a signal is assigned a project assuming the signal doesn't also come from a group, by default it assigns the contact. This means every other message from that contact is for that project. The signals screen updates in real time with this. So if we have 3 messges from Dave, if we switch one of them to be for a project, all the signals update to be for that project too. If the signal has been sent from a specific person, there are no other rules at play.

  - Assigning Groups to Projects. If a signal comes in from a contact and the signal has some kind of group attached (in Slack this would be the channel, in Email it might be the group of people the email is CCed to, in WhatsApp it'd be the group) then by default the project is assigned to the group. There will be a button in the UI to toggle between assigning a group and assigning the contact.

  - Assigning Contact and Group combination to projects. If a signal comes from a group, there's a third mode - assign this combination of contact and group to the project. This means every other message from this contact within this group is assigned to that project.

  **Examples**

  - Mike, my brother, is family. He messages in a group chat called "Mum's Chat" about care for my elderly mum. My brother Pete also replies in there. Everything in this group is assigned to the project "Support Mum" so all these messages are classed as for this project. Separately, we have a holiday chat which has Mike in it, but a bunch of other people who I don't really want notifications from. In this case, I want to assign the combination of Mike and this group to the "Holiday" project. Finally, if I get a message from Mike outside a group, I want a notification in the "Family" project. This shows the power of the specificity rules.

- Assignment of importance - every signal can either be important or not. This is because it's difficult to say if any signal is urgent or not ahead  of time. by default every signal is not important. If a signal from a sender is marked as important, every signal from that sender is also important.

- Notifications - every signal that's marked as important is shown to the user within the schedule allotted. For example - Mike is an important contact. He's attached to the Family project which is assigned to evenings and weekends. Mike messages me at 2pm on a Monday. This is not evening or weekend so I don't get the notification. However, when the evening schedule starts (maybe 7pm on Monday), this notification is triggered.

- Batching Out Of Schedule - when notifications are triggered out of schedule, notifications are stored up until the schedule is hit. At the point where the schedule starts, all these notifications are triggered. If there are more than a threshold (let's say 5), a single notification is sent per project. This way the user doesn't get overwhelmed with loads of notifications when the evening or weekend comes.

- Batching Within Schedule - by default, if notifications are triggered within schedule and they are for a person or group who are marked as important, then they immediately pop up on all devices. However, that can be disruptive. There will be a "Focus Mode On Demand" that allows you to focus for 1-4 hours. Once the focus mode is off, the notifications will come in. There's also a "Focus Mode By Default" mode - this puts you into focus mode for that project at all times, until you tell the software "Take a break from focus mode for 15 minutes" and you can deal with as many of the notifications as possible within that timeframe. The notifications are batched up ready for you to categories and / or deal with.

- Tasks - instead of seeing notifications and tasks as silos, we realise that often Tasks are created from notifications. There's a button alongside each signal. This allows me to create a task from the signal. There's also an option to assign the task to today's intentions.

### Putting It All Together - Example of Business Rules In Action

It's a Monday afternoon. I have three projects - Work, Mum and Friends.

Work has a schedule of 9-6pm. Mum has a schedule of 12-1pm - lunch time. Friends has a schedule of 6pm - 9pm.

I was added as the reviewer of a PR by my co-worker Steve at 10:00am and another at 10:30am.

I'm sent a text message from Elizabeth about Mum. Then Mike messages 3 times in the Mum Chat channel. Pete replies to him 6 times. This happens at 2pm - 3pm.

From 1pm until 7pm a group I'm in for the Humanists, a social group I'm a part of has 23 messages in it. There's one person in the group - Dave - who I get along with really well, but I don't know anyone else from that group too well. I want to build a friendship with that one person in the group.

I've also been sent an important email from my manager Stuart, which is mixed in with 28 other emails from newsletters and generic email notifications from Jira. That all happened between 11:34 and 2pm.

Also mixed into the Jira notifications is an email saying I was mentioned in a ticket comment by my team lead, Charlie. This happens at 7:30pm.

**How this is dealt with in Notify**

By default, I have no projects, so I'd go to the projects setup screen and create my three projects - Work, Mum and Friends.

I assign the Work project to be Business Hours schedule. The Mum project to be Lunch time. And the Friends project to be Evenings and Weekends - two different schedules.

Next, I look at the main screen - Signals. I see hundreds of signals that have come in since the last time I assigned rules. I scroll through them. The app doesn't have any idea what any of these things are yet, but I have some handy filters at the top to look for certain patterns such as "App is WhatsApp" or "By Frequency of Message From Contact" for example.

I see the signals for PR reviews. They're from Steve. I work in a team with Steve, and I want any kind of PR review where I'm tagged to be a notification. I press alongside the signal to switch this to "1W" which indicates it's for the work project. I can toggle between "Just Steve" and "Any PR Review". I toggle it to "Any PR review". Immediately in the interface I see that all PR review signals are assigned to "1W". I realise the implications of this in real time. I can then toggle the rule i've just made back to "Just Steve" and see all the other signals go back to be no project. 

## User Interface



## Classes of Notification

Fall into the Eisenhower matrix:

1. Urgent and Important - Notify me about these immediately on all platforms I'm on (mobile and desktop). Examples: Meeting in 10 minutes. Alarm. Outage in production. Timescale that requires a response from me - 5 - 15 minutes.

2. Urgent not Important - Urgent things to other people. Batch these notifications so that I can look at them on a schedule that makes sense for me. Some of these can be converted into TODOs. Others can be dealt with immediately or ignored.

3. Not Urgent and Important - Notifications that can be classified as activities that need to be scheduled. Or could be ignored.

4. Not Urgent or Important - Distractions. Block all these.



## Solution



## Example

An email from Meghan, our therapist, comes in.

A rule in the notify app is that any email from Meghan is marked as being *either* Urgent and Important or Not Urgent and Important. Either way, it's definitely important.

The email might be only important - "Hey Clare and John, here's a worksheet" - or it might be Urgent and Important - "Hey Clare and John, I need to hear back from you in the next day to hold your place next week".

This notification is attached to the Relationship project.

Because it *could* be urgent, it's flagged as relevant to me.

However, the notification rule is attached to the "evenings" schedule.

I don't want to be interrupted by this within the daytime as I'm working.

It also means I'm distracted from my daytime work.

So when I finish my work at 6pm I get a notification related to the email from Meghan.

I open the notification. I can create a TODO from the notification, which is sent to Intend or any other TODO app.

If I decide to not do anything, I can snooze the urgent and important notification until tomorrow.

But it only allows me to snooze one time. After that I need to take an action now or add it to Intend.


## Philosopy

This app is a way of dealing with notifications that fits with the Intend philosophy, outlined below.

A notification or group of notifications can be converted into an intention for today if it's Urgent and Important.  Or it can be deferred until tomorrow's intentions.  But no later than that - it needs to be done today or tomorrow.

For Urgent not important notifications, these will mostly be ignored or deleted by users, but someone can still create intentions for today from them.

For Important Not Urgent notifications, we can schedule them on a Google calendar.

There will be some kind of time blocking aspect - so each project will have certain schedules on which I can get work done on them.



## Intend

Intend has a similar philosophy to Notify and should be integrated fairly early on.

https://intend.do/

It has an API:

https://intend.do/features#api

Here's the philosophy:

Intend is an intentionality app, and it's also a philosophy/paradigm.
This page, by founder Malcolm Ocean, outlines that paradigm.

Intend helps people realize what their goals are, and make their goals a reality.

Virtually all to-do list software on the internet, whether it knows it or not, is based on the GTD philosophy (David Allen's "Getting Things Done") or some similar underlying assumptions.

The main paradigmatic differences of Intend, compared to GTD-based systems are as follows:

choosing & doing, over organizing
aliveness, instead of exhaustiveness
goals as fundamental, rather than tasks
proactive, rather than reactive
Keep reading and we'll explore each of them...

Main actions: choosing & doing (vs organizing)
Here are some taglines from various to-do list sites:

rememberthemilk.com: "The best way to manage your tasks"
todoist.com: “...the world’s most powerful to-do list. Access tasks anywhere”
any.do: “The World's Favorite Task Management App”
“Toodledo is an incredibly powerful tool to increase your productivity and organize your life.”
“If you like making to-do lists, you will love TeuxDeux.”
Evernote: "Tame your work, organize your life"
All of these either talk about organizing tasks or “to-do lists”.

Intend is, by many appearances, a "to-do list app", in the sense that it is an app (✓) where you make lists (✓) of things you intend to do (✓). But with Intend, the focus is on doing things, not on making and organizing lists.

The main way that the app currently embodies this philosophy is by not offering any place in the app to write down a bunch of stuff that you're not planning to work on yet and may never work on. It's not that we think such lists are not valuable—they are. But they have costs, and one of those costs is that people get too focused on keeping the list organized, at the expense of focusing on what they're actually trying to achieve and taking actions towards achieving those things.

A famous quote from Dwight D. Eisenhower:

“In preparing for battle I have always found that plans are useless, but planning is indispensable.”

Currently Intend essentially encourages you to plan elsewhere, so you still do the act of planning, but when you come back to Intend, you aren't faced with a bunch of old plans that are now in your way (worse than useless). You can also do some strategic thinking in your weekly/monthly/quarterly/yearly reviews, but it doesn't turn into a giant list of tasks.

More on this in the next section...

Emotional draw: aliveness (vs exhaustiveness)
I sometimes characterize the opposite of aliveness as staleness, as in “is your productivity system full of stale tasks?” but I’ll be charitable here and talk about the positive framing of GTD in this respect, which I’m going to call exhaustiveness. The first Step of GTD is Capture. Capturing things is important for getting them out of your working memory so you can focus on your work. But in my experience (and based on talking with people) it has issues as well:

the to-do list has more inflow than outflow, meaning it gets longer and longer
lots of low-value tasks get added and never removed, although they may still create guilt — even though it is not actually worth ever doing them!
when the person gains a new understanding of how it makes sense to approach that project/goal, old obsolete tasks don’t get cleared
(they may not even realize there's a contradiction between their new understanding and their old tasks)
The result of these issues tends to lead to the person keeping a separate list for newly emerged tasks that are of clearly higher-value than the original list, either within the same context or perhaps by starting to use a new app. The old “trusted system” is no longer trusted, because it’s full of stuff the person knows (at least implicitly) they don’t want to do, so they (reasonably) don't want to use it.

In principle, most of these failure modes can be combated with an effective weekly review that pares down the lists. In practice, almost nobody I've met actually consistently does the GTD weekly review (which means they aren't actually following the GTD system, but a hollow shell of GTD that nobody ever claimed would work).

One of RememberTheMilk’s taglines, as of this writing, is “Never forget the milk (or anything else) again.” The idea here is that nothing escapes the system—you put things in, and you can keep track of them, and not forget them. This is great, but some things actually are worth forgetting. Or worth ignoring. (Not to mention that if your list gets too long you'll end up forgetting about things anyway)

Intend currently has two main ways of prioritizing aliveness over exhaustiveness.

You can’t put in tasks for the future.
The only futurey thing you can do is to state a single top priority for each goal, with an optional check-in date. Having a single top priority makes it more likely that this priority will be something the person is excited to work on, or that they at least feel is high-value.
Intend doesn't assume that a task left undone today is something you want to do tomorrow.
With most apps, if you don’t do something one day, it just sticks around. Sometimes this is vital, but often it mostly just increases the sense of guilt and burden around the task, undermining the unconscious intelligence that often keeps us from busywork. Intend does have a system for grabbing yesterday's notdones for today, which prompts you to reflect on whether they're still important and how things will go differently.
Future implementations of Intend may also have a system for brainstorming potential future tasks, or breaking down big things into small tasks, but without the assumption that all of those necessarily will be completed eventually. Instead, the brainstormed list would act as inspiration for choosing one’s daily intentions, and tasks from it that are ignored would gradually be set aside, automating the process whereby the user makes a new list when the old stuff becomes stale.

Since most people have at least a couple goals where there is some sense of “things I might do in the future,” they end up using other systems to keep track of those. Perhaps these users will use those apps totally effectively, but importantly... with an organizer+Intend combo, even if the organizer goes stale, Intend will still be there, asking you what’s most important to do today. That might be some object-level things that are obvious, or it might be the task “purge my old task list” or even “switch from using workflowy to track future tasks to todoist”.

It seems to me that an aliveness-based system works better than an exhaustiveness-based system for people who are pursuing purposeful goals (personal or professional) where they get to discern what their priorities are and where most small tasks are not critical. Where small tasks are critical, other systems (including email inboxes) can supplement Intend for ensuring those are taken care of. An administrative or personal assistant would not want to use Intend to keep track of the tasks assigned by their employer. Many Intend users are students, self-employed, and/or freelancers. Of those who are employed, many have substantial control over how they approach their work assignments, and those who do not tend not to track their work-related tasks in Intend.

Main nouns: goals (vs tasks)
Many other to-do list apps feature, in their product demos, people buying groceries, and listing out each grocery they need. While this is a legitimate thing to want to build an app for, it's a completely different use-case from Intend.

While GTD-based systems have projects, the projects mostly are just buckets to put tasks into. Tasks exist as freefloating entities that might not be associated with a project at all, or might be in theory associated with a project but when you put the task in your inbox you didn’t assign it to a project, so you first need to process your inbox, etc.

With Intend, goals come first. You literally can't access the rest of the app before first setting at least two goals that you’re working towards. Most users have 3-6. You get up to 10, each with its own digit, so you have a goal 1, goal 2, etc).

Then, when you go to enter your intentions for the day, you have to explicitly indicate which goal it is for using the goal's number, or you use an ampersand to indicate that it's a miscellaneous intention that isn't associated with one of your goals. This means that it is always really clear why you’re doing what you’re doing. That’s not to say that you will necessarily do the most strategic thing towards that goal, but it does make it more likely that you will do something rather than nothing, and also much more likely that you will notice that what you’re doing isn’t really strategic, because:

at the end of each day, in the outcomes, you indicate if you did enough towards each goal such that if all other days of this reference class went like this one did, you’d be on track to achieving your goal
if you do weekly reviews, then you get another chance to notice that maybe you’re not actually on track—that after watching 3 seasons of a cooking show you're still ordering takeout and don't own a frying pan
I think that having goals come first means that users are more likely to forget to do random small tasks and less likely to forget to make progress towards their high-level goals, which I see as being probably a good tradeoff, especially if the person has other systems in place to ensure small tasks don’t get forgotten if they’re indeed really important.

Approach: proactive (vs reactive)
This section is mostly implied by the above sections, but it's worth pointing at directly. Essentially the distinction here is:

GTD-based systems say “here is all of this stuff I could do… what shall I do?”
Intend asks “what do I want to achieve… how can I achieve it?”
This represents a difference between reactively prioritizing incoming demands or proposals (from coworkers or email newsletters) or random impulses, and proactively seeking ways to reach a goal. If you're doing anything self-directed or creative, you want your energy coming from within, scaffolded by a context that reminds you what you care about.

In our Beyond Goals Intensive workshops, we encourage people, once they've set some new goals, to come up with a thing to do towards each goal immediately (today or tomorrow) that they wouldn't have thought to do at all prior to having the goal. This is another way to point at proactive vs reactive approaches.

Proactively pursuing a goal doesn't automatically imply strategically pursuing a goal, but for most people the bottleneck isn’t unstrategicness but lack of intentional momentum towards goals at all. So Intend is mostly focused on momentum. You can't be strategic without something to be strategic towards! And to the extent that you're genuinely trying to achieve something and regularly assessing how that's going, you'll tend to develop or seek out better strategies.

There's some science on this—if you're curious, you can check out the Mechanisms section of Building a Practically Useful Theory of Goal Setting and Task Motivation: A 35-Year Odyssey, a paper that summarizes decades of research into how goals affect performance.


The concept of “productivity” was invented in the late 1800s as a frame for measuring standardized output among laborers all doing the same job. It’s since been reapplied to refer to any kind of output, including creative or intellectual work, but it still only asks: what is produced?

If you try to maximize productivity, any success comes at the expense of eliminating what makes life worth living: play, rest, health, serendipity, social connection, or even the extra care involved in adding those unnecessary touches to your work, that delight you and others.

More and more people are noticing that productivity isn’t really what we want.

It seems to me that what many of us want is intentionality.

Intentionality is not about output, but about the connection between what you’re doing and why you’re doing it. It’s not about working as much as you can, but about fully working when you’re working, fully playing when you’re playing, and fully resting when you’re resting.

It feels good to do good work. We get into flow states—we dive deep into the problems we’re solving and see ever more clearly the landscape of the system we’re sculpting. Focused work is viscerally engaging: we talk excitedly about tackling or grappling with a problem.

It feels good to relax into rest. We get lost in a novel or a forest or our loved one’s eyes, trusting that whatever we’ll need to take care of tomorrow or next week, we will take care of that then. Pure rest is wondrously expansive: we talk about being rejuvenated—literally “made young again”.

It feels good to play. We get the hang of a new game, we get to know each other better in surprising ways, and we get around to tinkering with a project we’d always wanted to poke at, not really sure where it’s going and not really caring. Play is open-endedly serendipitous 🙃

What feels awful is when we don’t know what the fuck we’re doing. When we’re supposed to be working but we’re fighting a distracting environment or internal resistance. When we can’t relax during “time off”, because of looming deadlines or feeling we haven’t done enough.

Intentionality is about knowing what you’re doing at a given moment and doing it. It’s about being present to your actions and feeling the satisfaction as you allow yourself ever more fully into doing what you love doing, whether work, play, or rest.

Intentionality is about noticing when your intentions conflict: when doing what you set out to do isn’t quite so simple, because some other part of you has other plans. It’s about finding a new approach where you don’t have your foot on the gas and the brake at the same time.

Intentionality is about connecting the dots between the actions you’re taking right now, in each moment, and the big picture future you want to live in, that those actions are helping bring about. It’s about seeing exciting new opportunities appear as you get clearer where you’re going.

Intentionality is about experiencing whatever you're doing as part of a learning process that will inform the rest of your life.

A productivity system has tasks—often more than you can remember. Within an intentionality system, you have intentions. Intending is internal & embodied—unlike tasks, intentions don’t exist “out there” somewhere. The system just helps you clarify what it is you’re intending to do.

A good intentionality system therefore needs to also have a lifecycle by which things you intended to do awhile ago but haven't done get composted to make fertile soil for new fresh intentions, rather than building up a pile of stale tasks.

It's not that productivity is bad—an intentionality system will probably make you more productive as well! Just... not as an automaton.

