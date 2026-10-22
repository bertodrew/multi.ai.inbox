# 04 · Branding

Part A documents the reference brand as observed, for learning only. Part B defines a distinct identity for multi.ai.inbox. We do not reuse AgentMail's name, logo, illustrations or copy.

## Part A · AgentMail brand system [observed]

| Element | Value |
|---------|-------|
| Name / wordmark | "AgentMail", geometric sans wordmark; symbol is a spy style hat and mask over a chevron (agent as secret agent pun) |
| Color scheme | Dark first |
| Background | `#09090B` (near black, zinc 950) |
| Primary accent | `#FF6600` signal orange (envelopes, dots, highlights) |
| Secondary accent | `#1454ED` electric blue (links, active tab) |
| Supporting | beige / kraft paper `#C9B49A` range, pale blue envelope, white text |
| Heading font | custom `marketingSans` (fallback Funnel Sans), H1 56 px, H2 40 px |
| Body font | Funnel Sans / Inter, 16 px |
| Mono font | JetBrains Mono, SF Mono; used for eyebrow labels in uppercase with wide tracking ("PRODUCT", "SECURITY AND CONTROL") |
| Radius | 6 px buttons, 8 px inputs, larger rounded cards on logo bar |
| Grid | 8 px base unit |
| UI kit | Tailwind + shadcn/ui on Next.js (`/_next/image`, Vercel `dpl_` ids) [inferred] |
| Motifs | Physical envelopes and paper, wireframe globes and spirals, coordinate labels like `(0.4, 0.1)`, dotted world maps, faint pictogram grid in hero background |
| Voice | Short declarative headlines with a twist ("We're not AI for your email, we're email for your AI", "Independence needs boundaries", "Gmail was built for Humans, AgentMail was built for Agents"); developer audience; speaks to agents directly via copyable prompts |
| Social proof style | Monochrome logos on a dark card, metrics as big numerals ("1B+", "10M+") |

Takeaways we keep as **principles**, not assets:
1. Make the abstract tangible (an inbox as a physical object).
2. Dark technical canvas with one loud accent color.
3. Mono uppercase eyebrows for structure.
4. Talk to the agent as a user (copyable prompt, `llms.txt`).

## Part B · multi.ai.inbox identity [decision]

### Positioning line
"Inboxes for AI agents. Hosted in Europe."
Alternates: "Your agents get an address. Your data stays in the EU." / "Email your agents can own."

### Brand idea: **the post office box**
Where AgentMail uses envelopes, we use the **P.O. box / mail slot grid**: a wall of numbered brass boxes, one per agent. It conveys many inboxes, ownership, and a European post office heritage. Each box can glow when mail arrives (motion cue for events and webhooks).

### Palette

| Token | Light | Dark | Use |
|-------|-------|------|-----|
| `--bg` | `#F7F6F2` (paper) | `#0D1117` (ink) | page background |
| `--fg` | `#14161A` | `#E8E6E1` | text |
| `--muted` | `#6B6F76` | `#8B9099` | secondary text |
| `--card` | `#FFFFFF` | `#151A22` | surfaces |
| `--primary` | `#1F5BFF` EU blue | `#4C7DFF` | primary buttons, links |
| `--accent` | `#F2B705` brass yellow | `#F2C230` | the "mail arrived" signal, highlights |
| `--success` | `#1E9E62` | `#35C281` | verified DNS, delivered |
| `--danger` | `#D93A3A` | `#FF5C5C` | bounced, blocked |

Blue plus yellow nods to the EU flag without copying it. Contrast: all text tokens meet WCAG AA on their background.

### Typography (free, Google Fonts)
* Headings: **Space Grotesk** 600, tight tracking.
* Body: **Inter** 400 and 500.
* Mono and eyebrows: **JetBrains Mono** 500, uppercase, letter spacing 0.08em.
Scale: 14 / 16 / 20 / 28 / 40 / 56 px.

### Logo concept
A rounded square split into a 2 × 2 grid of mail slots; the top right slot is brass yellow (one agent just got mail). Wordmark "multi.ai.inbox" in Space Grotesk with the dots in accent yellow. Favicon is the grid alone.

### Voice and tone
* Plain, precise, a bit dry. No hype words ("revolutionary", "magic").
* Developer first: show code before prose.
* Always state where data lives ("Stored in Falkenstein, Germany").
* Copy rule from the founder: no dash characters as punctuation in UI text; use commas, colons or periods.

### Illustration and motion
* Isometric P.O. box walls, numbered slots, paper tags with mono labels (`INBOX 0042`).
* Motion: a slot lights up and a tag slides out when an event fires; respect `prefers-reduced-motion`.

### Implementation
Tokens live as CSS variables in `src/app/globals.css`, mapped into Tailwind and shadcn/ui themes; light and dark via `data-theme` plus `prefers-color-scheme`. Radius 8 px everywhere, 12 px for cards.
