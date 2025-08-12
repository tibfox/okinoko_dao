# DAO Contract – User Guide

This smart contract powers community-led projects on the VSC network.  
It allows people to create projects, join them, make proposals, vote on them, and manage shared funds — all in a transparent, on-chain way.

---

## 1. What You Can Do

- **Create a Project**  
  Start your own community project with a name, description, voting rules, and a shared treasury.  
  You decide:
  - Who can make proposals (just you, or all members)
  - How members join (fixed fee for equal voting or stake-based for weighted voting)
  - The percentage of votes needed for approval
  - Proposal cost (goes into project funds)
  - Proposal duration
  - Minimum/Exact join amounts
  - Optional: Enable/disable features like reward distribution, secret voting, and more

- **Join a Project**  
  Become a member by sending the required join amount (set by the project).  
  - In **Democratic voting** projects, every member’s vote counts equally.  
  - In **Stake-based voting**, your vote weight depends on your contribution amount.

- **Make a Proposal**  
  Suggest an action or ask the community a question.
  Proposal types:
  - **Yes/No** (can also execute fund transfers if approved)
  - **Single Choice Poll**
  - **Multiple Choice Poll**
  Every proposal has:
  - Title, description, and extra metadata (for future features)
  - Duration for voting
  - Receiver (only for Yes/No fund transfers)
  - Cost (defined by the project, goes into treasury)

- **Vote on Proposals**  
  Members vote according to the project’s rules.  
  The system calculates results automatically after the deadline.

- **Send Additional Funds**  
  You can add more funds to the project’s treasury at any time to help the community achieve its goals.

---

## 2. How Voting Works

1. **Democratic Voting** – Every member has **1 vote**, regardless of stake.  
2. **Stake-based Voting** – Your vote weight = the amount you staked when joining.  

Projects can set:
- Minimum/Exact join amount
- Percentage needed to pass a proposal
- Who is allowed to create proposals
- If proposals require a cost

---

## 3. Proposal Results

When voting ends:
- If **Yes/No** and passes → Funds are sent (if applicable).  
- If a poll → Results are recorded on-chain for everyone to see.  
- The project treasury is updated automatically.

---

## 4. Commands & Actions

From your wallet or UI, you can:

| Action            | Description |
|-------------------|-------------|
| `create_project`  | Start a new project |
| `join_project`    | Become a member by paying the join amount |
| `create_proposal` | Suggest an action/question |
| `vote`            | Cast your vote |
| `get_projects`    | View all projects |
| `get_proposals`   | View all proposals for a project |
| `get_results`     | See how proposals ended |
| `send_funds`      | Add more to the treasury |

---

## 5. Tips for Success

- Make your **proposal descriptions clear** so members know exactly what they are voting for.
- Always check the **voting deadline** before you submit your vote.
- If joining a stake-based project, your **initial stake matters** — it defines your voting power.
- Proposal costs go to the project treasury, so even failed proposals contribute to the community.

---

## 6. Safety

- All votes and results are public and stored on-chain.  
- The project owner can hand over control to another member via a special transfer function.  
- Only members can vote, and only according to the project’s rules.
