# Formula Editor User Guide

This guide helps non-technical users create, edit, and test calculation formulas using the Insurance Formula Engine's visual editor. No programming experience required.

---

## 1. Interface Overview

When you open the formula editor, you'll see the following layout:

![Visual Editor Overview](images/02-visual-editor.png)

| Area | Location | Description |
|------|----------|-------------|
| **Header** | Top | Formula name, category, version, mode toggle (Visual / Text), Save button |
| **Node Palette** | Left sidebar | Draggable list of available node types |
| **Canvas** | Center | Visual editing area where you build formulas |
| **Properties Panel** | Right sidebar | Shows configuration for the selected node |
| **Test Panel** | Bottom | Enter test inputs and run calculations |

---

## 2. Node Types

Formulas are built from different types of "nodes," each representing a calculation element:

| Icon | Name | Purpose | Example |
|------|------|---------|---------|
| `x` | **Variable** | Formula input parameter | Age, premium, rate |
| `#` | **Constant** | Fixed numeric value | 1, 100, 3.14 |
| `+-` | **Operator** | Arithmetic operations | Premium × Rate |
| `f(x)` | **Function** | Mathematical functions | round, sqrt, abs |
| `{}` | **Sub-formula** | Reference another formula | Reference "Base Rate Calc" |
| `[]` | **Table Lookup** | Look up values in a data table | Find mortality rate by age |
| `?` | **Conditional** | Choose values based on conditions | If age ≥ 65 then… else… |
| `Σ` | **Aggregate** | Combine multiple values | Sum, average |
| `↺` | **Loop** | Repeat calculations over a range | Sum for t = 1 to n |

---

## 3. Basic Operations

### 3.1 Creating Nodes

1. Find the node type you need in the **Node Palette** on the left
2. **Click and drag** it onto the canvas
3. Release the mouse button — the node is created

### 3.2 Connecting Nodes

Nodes are connected through **ports** to form a calculation flow:

- Each node has an **output port** (Out) on the right side
- Each node has one or more **input ports** on the left side (e.g., Left, Right)
- **Drag** from one node's output port to another node's input port to create a connection

**Rules:**
- Data flows from left to right (inputs → processing → output)
- A node cannot connect to itself
- Each input port accepts only one connection

### 3.3 Editing Node Properties

1. **Click** a node on the canvas to select it
2. The **Properties Panel** on the right shows its configuration
3. Modify the settings based on the node type

![Node Properties Panel](images/03-node-selected.png)

**Properties by Node Type:**

| Node Type | Properties to Set |
|-----------|-------------------|
| Variable | Name (e.g., `age`), data type |
| Constant | Value (e.g., `100`) |
| Operator | Operation: Add(+), Subtract(-), Multiply(×), Divide(÷), Power(^), Modulo(%) |
| Function | Function name: round, floor, ceil, abs, min, max, sqrt, ln, exp |
| Sub-formula | Select formula to reference, optionally pin a version |
| Table Lookup | Select table, set key columns, set output column |
| Conditional | Comparison operator: ==, !=, >, >=, <, <= |
| Loop | Body formula, iterator variable, aggregation method |

### 3.4 Deleting Nodes or Connections

1. **Click** the node or connection line to select it
2. Press the **Delete** key on your keyboard

### 3.5 Auto Layout

If nodes look messy, click the **"Auto Layout"** button above the canvas to automatically arrange them.

---

## 4. Building a Formula: Step-by-Step Example

### Example: Calculating BMI (Body Mass Index)

BMI = Weight ÷ Height²

**Steps:**

1. Drag a **Variable** node onto the canvas, name it `weight`
2. Drag another **Variable** node, name it `height`
3. Drag a **Constant** node, set value to `2`
4. Drag an **Operator** node, select "Power(^)"
   - Connect `height` to the Left port
   - Connect constant `2` to the Right port
5. Drag another **Operator** node, select "Divide(÷)"
   - Connect `weight` to the Left port
   - Connect the power result to the Right port

The canvas now shows a left-to-right calculation flow.

---

## 5. Advanced Features

### 5.1 Sub-formula References

When formulas become complex, you can split them into smaller sub-formulas and reference them from the main formula.

1. Create and save the sub-formula first
2. In the main formula, drag a **Sub-formula** node
3. Select the formula to reference in the Properties Panel

### 5.2 Table Lookups

Look up values from predefined data tables (e.g., rate tables, mortality tables).

1. Drag a **Table Lookup** node
2. Select the data table in the Properties Panel
3. Set the key column (used to locate the row)
4. Set the output column (the value to retrieve)
5. Connect the key value to the input port

### 5.3 Conditional Logic

Choose different calculations based on conditions.

1. Drag a **Conditional** node
2. Set the comparison operator (e.g., `>=`)
3. Connect four input ports:
   - **If**: The value to compare (e.g., age)
   - **Cmp**: The threshold (e.g., constant 65)
   - **Then**: Value if condition is true
   - **Else**: Value if condition is false

### 5.4 Loops

Repeat calculations across a range of values — a core feature for actuarial calculations.

![Loop Node Example](images/05-annuity-visual.png)

**Basic Loops (Sum / Product):**

1. Create the loop body as a separate sub-formula and publish it
2. In the main formula, drag a **Loop** node
3. Set its properties:
   - **Body Formula**: Select the sub-formula
   - **Iterator Variable**: Counter name used in the body (e.g., `t`)
   - **Aggregation**: sum, product, avg, min, max, etc.
4. Connect the Start and End ports (loop range)

**Fold Loops (Recursive Accumulation):**

For calculations where each step depends on the previous result (e.g., reserve recursion).

![Fold Loop Example](images/06-reserve-fold.png)

1. Drag a Loop node, set Aggregation to **Fold**
2. Set the **Accumulator Variable** (e.g., `V`) and **Initial Value** (e.g., `0`)
3. The body formula uses `V` (previous result) and `t` (current step)

---

## 6. Text Editing Mode

In addition to the visual editor, you can write formulas as text.

![Text Editor](images/04-text-editor.png)

### 6.1 Switching to Text Mode

Click the **"Text Editor"** tab at the top of the page.

### 6.2 Text Syntax

| Syntax | Meaning |
|--------|---------|
| `a + b` | Addition |
| `a * b` | Multiplication |
| `a / b` | Division |
| `a ^ 2` | Power |
| `round(x, 2)` | Round to 2 decimal places |
| `sqrt(x)` | Square root |
| `if a >= b then x else y` | Conditional |
| `lookup("tableName", key)` | Table lookup |
| `subFormula("formulaID")` | Sub-formula reference |
| `sum_loop("formulaID", t, 1, n)` | Sum loop |
| `product_loop("formulaID", t, 1, n)` | Product loop |
| `fold_loop("formulaID", t, 0, n, V, 0)` | Fold loop |

### 6.3 LaTeX Preview

Below the text input, a **mathematical preview** renders automatically.

For example, `sum_loop("body", t, 1, n)` displays as:

$$\sum_{t=1}^{n} \text{body}(t)$$

### 6.4 LaTeX Input Mode

Click the **"LaTeX"** tab to type LaTeX expressions directly. The system converts them to formula text automatically.

### 6.5 Apply to Graph

After editing in text mode, click **"Apply to Graph"** to convert the text back into a visual graph.

### 6.6 Text Mode Limitations

Some node types do not yet round-trip through the text editor. If your
formula contains any of these, you should stay in **Visual Editor** mode:

- **Loop nodes** (`sum_loop` / `product_loop` / `fold_loop` / etc.):
  some loop configurations (`inclusiveEnd`, `maxIterations`, `version`)
  are visual-only and cannot be expressed in text. An inline notice
  appears when the limitation kicks in.
- **Composite Conditional nodes** (multi-term `AND` / `OR` / `NOT`,
  added in task #039): the text grammar has no boolean keywords yet, so
  a formula that uses composite conditionals can only be edited in the
  visual editor — switching to text mode will show an explicit error
  directing you back.
- **TableAggregate nodes** (SQL-style aggregate over a lookup table,
  added in task #040): the text grammar has no SQL aggregate syntax
  yet. The seeded `日本損害保険 チェインラダー LDF` formula is the
  first in-tree user; switching it to text mode shows an explicit error
  directing you back to the visual editor.

The full list of engine known limitations (including statistical
distribution functions, date arithmetic, cross-row table aggregation,
and per-call statelessness) is documented in the project
[`README.md` § Known Limitations](../../README.md#known-limitations).

---

## 7. Testing and Calculating

### 7.1 Enter Test Inputs

In the **Test Panel** at the bottom, enter variable values in JSON format:

```json
{"age": "35", "sumAssured": "1000000"}
```

### 7.2 Run the Calculation

Click the **"Calculate"** button. The engine will execute the formula with your inputs.

### 7.3 View Results

Results appear next to the button in JSON format.

---

## 8. Saving and Version History

### Saving

Click the **"Save"** button in the top right corner. The system validates first:
- **Red message**: Errors found (e.g., missing connections)
- **Green "Saved"**: Successfully saved

### Version History

Each save creates a new version. Click **"Version History"** in the header to view all past versions.

---

## FAQ

**Q: Why does saving show an error?**
Check that all required input ports are connected. Common issue: operator nodes missing Left or Right inputs.

**Q: Why does my loop node show an error?**
Verify: (1) Body formula is selected (2) Iterator variable name is set (3) Start and End ports are connected.

**Q: Why does text mode show extra outputs?**
Text mode displays unconnected variable nodes as separate outputs. This is normal — during calculation, these variables receive values from the input parameters.
