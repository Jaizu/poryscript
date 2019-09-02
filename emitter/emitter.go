package emitter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/huderlem/poryscript/token"

	"github.com/huderlem/poryscript/ast"
)

// Emitter is responsible for transforming a parsed Poryscript program into
// the target assembler bytecode script.
type Emitter struct {
	program  *ast.Program
	optimize bool
}

// New creates a new Poryscript program emitter.
func New(program *ast.Program, optimize bool) *Emitter {
	return &Emitter{
		program:  program,
		optimize: optimize,
	}
}

// Emit the target assembler bytecode script.
func (e *Emitter) Emit() string {
	var sb strings.Builder
	i := 0
	for _, stmt := range e.program.TopLevelStatements {
		if i > 0 {
			sb.WriteString("\n")
		}

		scriptStmt, ok := stmt.(*ast.ScriptStatement)
		if ok {
			sb.WriteString(e.emitScriptStatement(scriptStmt))
			i++
			continue
		}

		rawStmt, ok := stmt.(*ast.RawStatement)
		if ok {
			sb.WriteString(emitRawStatement(rawStmt))
			i++
			continue
		}

		fmt.Printf("Could not emit top-level statement because it is not recognized: %q", stmt.TokenLiteral())
		return ""
	}

	for j, text := range e.program.Texts {
		if i+j > 0 {
			sb.WriteString("\n")
		}

		emitted := emitText(text)
		sb.WriteString(emitted)
	}
	return sb.String()
}

func (e *Emitter) emitScriptStatement(scriptStmt *ast.ScriptStatement) string {
	// The algorithm for emitting script statements is to split the scripts into
	// self-contained chunks that logically branch to one another. When branching logic
	// occurs, create a new chunk for any shared logic that follows the branching, as well
	// as new chunks for the destination of the branching logic. When creating and processing
	// new chunks, it's important to remember where the chunks should return to.
	chunkCounter := 0
	finalChunks := make(map[int]*chunk)
	remainingChunks := []*chunk{
		&chunk{id: chunkCounter, returnID: -1, statements: scriptStmt.Body.Statements[:]},
	}
	loopStatementReturnChunks := make(map[ast.Statement]int)
	loopStatementOriginChunks := make(map[ast.Statement]int)
	for len(remainingChunks) > 0 {
		ids := []int{}
		for _, c := range remainingChunks {
			ids = append(ids, c.id)
		}

		// Grab an unprocessed script chunk.
		curChunk := remainingChunks[0]
		remainingChunks = remainingChunks[1:]

		// Skip over basic command statements.
		i := 0
		shouldContinue := false
		for _, stmt := range curChunk.statements {
			commandStmt, ok := stmt.(*ast.CommandStatement)
			if !ok {
				break
			}
			// "end" and "return" are special control-flow commands that end execution of
			// the current logic scope. Therefore, we should not process any further into the
			// current chunk, and mark it as finalized.
			if commandStmt.Name.Value == "end" || commandStmt.Name.Value == "return" {
				completeChunk := &chunk{id: curChunk.id, returnID: -1, statements: curChunk.statements[:i]}
				finalChunks[completeChunk.id] = completeChunk
				shouldContinue = true
				break
			}
			i++
		}
		if shouldContinue {
			continue
		}

		if i == len(curChunk.statements) {
			// Finalize a new chunk, if we reached the end of the statements.
			finalChunks[curChunk.id] = curChunk
			continue
		}

		// Create new chunks from if statement blocks.
		if stmt, ok := curChunk.statements[i].(*ast.IfStatement); ok {
			newRemainingChunks, ifBranch := createIfStatementChunks(stmt, i, curChunk, remainingChunks, &chunkCounter)
			remainingChunks = newRemainingChunks
			completeChunk := &chunk{
				id:             curChunk.id,
				returnID:       curChunk.returnID,
				statements:     curChunk.statements[:i],
				branchBehavior: ifBranch,
			}
			finalChunks[completeChunk.id] = completeChunk
		} else if stmt, ok := curChunk.statements[i].(*ast.WhileStatement); ok {
			newRemainingChunks, jump, returnID := createWhileStatementChunks(stmt, i, curChunk, remainingChunks, &chunkCounter)
			remainingChunks = newRemainingChunks
			completeChunk := &chunk{
				id:             curChunk.id,
				returnID:       curChunk.returnID,
				statements:     curChunk.statements[:i],
				branchBehavior: jump,
			}
			finalChunks[completeChunk.id] = completeChunk
			loopStatementReturnChunks[stmt] = returnID
			loopStatementOriginChunks[stmt] = jump.destChunkID
		} else if stmt, ok := curChunk.statements[i].(*ast.DoWhileStatement); ok {
			newRemainingChunks, jump, returnID := createDoWhileStatementChunks(stmt, i, curChunk, remainingChunks, &chunkCounter)
			remainingChunks = newRemainingChunks
			completeChunk := &chunk{
				id:             curChunk.id,
				returnID:       curChunk.returnID,
				statements:     curChunk.statements[:i],
				branchBehavior: jump,
			}
			finalChunks[completeChunk.id] = completeChunk
			loopStatementReturnChunks[stmt] = returnID
			loopStatementOriginChunks[stmt] = jump.destChunkID
		} else if stmt, ok := curChunk.statements[i].(*ast.BreakStatement); ok {
			destChunkID, ok := loopStatementReturnChunks[stmt.LoopStatment]
			if !ok {
				panic("Could not emit 'break' statement because its return point is unknown.")
			}
			completeChunk := &chunk{
				id:             curChunk.id,
				returnID:       curChunk.returnID,
				statements:     curChunk.statements[:i],
				branchBehavior: &breakContext{destChunkID: destChunkID},
			}
			finalChunks[completeChunk.id] = completeChunk
		} else if stmt, ok := curChunk.statements[i].(*ast.ContinueStatement); ok {
			destChunkID, ok := loopStatementOriginChunks[stmt.LoopStatment]
			if !ok {
				panic("Could not emit 'continue' statement because its return point is unknown.")
			}
			completeChunk := &chunk{
				id:             curChunk.id,
				returnID:       curChunk.returnID,
				statements:     curChunk.statements[:i],
				branchBehavior: &breakContext{destChunkID: destChunkID},
			}
			finalChunks[completeChunk.id] = completeChunk
		} else {
			completeChunk := &chunk{
				id:         curChunk.id,
				returnID:   curChunk.returnID,
				statements: curChunk.statements[:i],
			}
			finalChunks[completeChunk.id] = completeChunk
		}
	}

	return e.renderChunks(finalChunks, scriptStmt.Name.Value)
}

func createConditionDestination(destinationChunk int, operatorExpression *ast.OperatorExpression) *conditionDestination {
	return &conditionDestination{
		id:                 destinationChunk,
		operatorExpression: operatorExpression,
	}
}

func createIfStatementChunks(stmt *ast.IfStatement, i int, curChunk *chunk, remainingChunks []*chunk, chunkCounter *int) ([]*chunk, *jump) {
	remainingChunks, returnID := curChunk.splitChunkForBranch(i, chunkCounter, remainingChunks)

	*chunkCounter++
	consequenceChunk := &chunk{
		id:         *chunkCounter,
		returnID:   returnID,
		statements: stmt.Consequence.Body.Statements,
	}
	remainingChunks = append(remainingChunks, consequenceChunk)

	elifChunks := []*chunk{}
	for _, elifStmt := range stmt.ElifConsequences {
		*chunkCounter++
		elifChunk := &chunk{
			id:         *chunkCounter,
			returnID:   returnID,
			statements: elifStmt.Body.Statements,
		}
		remainingChunks = append(remainingChunks, elifChunk)
		elifChunks = append(elifChunks, elifChunk)
	}

	var elseChunk *chunk
	if stmt.ElseConsequence != nil {
		*chunkCounter++
		elseChunk = &chunk{
			id:         *chunkCounter,
			returnID:   returnID,
			statements: stmt.ElseConsequence.Statements,
		}
		remainingChunks = append(remainingChunks, elseChunk)
	}

	// Stitch together the return ids for the cascading if statements in reverse order.
	prevElifEntryID := -1
	if len(elifChunks) > 0 {
		for i := len(elifChunks) - 1; i >= 0; i-- {
			if i == len(elifChunks)-1 {
				if elseChunk != nil {
					remainingChunks, _, prevElifEntryID = splitBooleanExpressionChunks(stmt.ElifConsequences[i].Expression, chunkCounter, elifChunks[i].id, elseChunk.id, remainingChunks, -1)
				} else {
					remainingChunks, _, prevElifEntryID = splitBooleanExpressionChunks(stmt.ElifConsequences[i].Expression, chunkCounter, elifChunks[i].id, returnID, remainingChunks, -1)
				}
			} else {
				remainingChunks, _, prevElifEntryID = splitBooleanExpressionChunks(stmt.ElifConsequences[i].Expression, chunkCounter, elifChunks[i].id, prevElifEntryID, remainingChunks, -1)
			}
		}
	}

	var initialEntryChunkID int
	if len(elifChunks) > 0 {
		remainingChunks, _, initialEntryChunkID = splitBooleanExpressionChunks(stmt.Consequence.Expression, chunkCounter, consequenceChunk.id, prevElifEntryID, remainingChunks, -1)
	} else if elseChunk != nil {
		remainingChunks, _, initialEntryChunkID = splitBooleanExpressionChunks(stmt.Consequence.Expression, chunkCounter, consequenceChunk.id, elseChunk.id, remainingChunks, -1)
	} else {
		remainingChunks, _, initialEntryChunkID = splitBooleanExpressionChunks(stmt.Consequence.Expression, chunkCounter, consequenceChunk.id, returnID, remainingChunks, -1)
	}

	return remainingChunks, &jump{destChunkID: initialEntryChunkID}
}

func splitBooleanExpressionChunks(expression ast.BooleanExpression, chunkCounter *int, successChunkID int, failureChunkID int, remainingChunks []*chunk, firstID int) ([]*chunk, *chunk, int) {
	if operatorExpression, ok := expression.(*ast.OperatorExpression); ok {
		dest := createConditionDestination(successChunkID, operatorExpression)
		*chunkCounter++
		newChunk := &chunk{
			id:             *chunkCounter,
			statements:     []ast.Statement{},
			branchBehavior: &leafExpressionBranch{truthyDest: dest, falseyReturnID: failureChunkID},
		}
		remainingChunks = append(remainingChunks, newChunk)
		if firstID == -1 {
			firstID = newChunk.id
		}
		return remainingChunks, newChunk, firstID
	}

	if binaryExpression, ok := expression.(*ast.BinaryExpression); ok {
		if binaryExpression.Operator == token.AND {
			*chunkCounter++
			successChunk := &chunk{
				id:         *chunkCounter,
				statements: []ast.Statement{},
			}
			var linkChunk *chunk
			var leftLink *chunk
			remainingChunks, leftLink, firstID = splitBooleanExpressionChunks(binaryExpression.Left, chunkCounter, successChunk.id, failureChunkID, remainingChunks, firstID)
			remainingChunks, linkChunk, firstID = splitBooleanExpressionChunks(binaryExpression.Right, chunkCounter, successChunkID, failureChunkID, remainingChunks, firstID)
			successChunk.branchBehavior = &jump{destChunkID: linkChunk.id}
			remainingChunks = append(remainingChunks, successChunk)
			return remainingChunks, leftLink, firstID
		} else if binaryExpression.Operator == token.OR {
			*chunkCounter++
			failChunk := &chunk{
				id:         *chunkCounter,
				statements: []ast.Statement{},
			}
			var linkChunk *chunk
			var leftLink *chunk
			remainingChunks, leftLink, firstID = splitBooleanExpressionChunks(binaryExpression.Left, chunkCounter, successChunkID, failChunk.id, remainingChunks, firstID)
			remainingChunks, linkChunk, firstID = splitBooleanExpressionChunks(binaryExpression.Right, chunkCounter, successChunkID, failureChunkID, remainingChunks, firstID)
			failChunk.branchBehavior = &jump{destChunkID: linkChunk.id}
			remainingChunks = append(remainingChunks, failChunk)
			return remainingChunks, leftLink, firstID
		}
	}

	return remainingChunks, nil, firstID
}

func createWhileStatementChunks(stmt *ast.WhileStatement, i int, curChunk *chunk, remainingChunks []*chunk, chunkCounter *int) ([]*chunk, *jump, int) {

	remainingChunks, returnID := curChunk.splitChunkForBranch(i, chunkCounter, remainingChunks)

	*chunkCounter++
	headerChunk := &chunk{
		id:         *chunkCounter,
		returnID:   returnID,
		statements: []ast.Statement{},
	}

	*chunkCounter++
	consequenceChunk := &chunk{
		id:         *chunkCounter,
		returnID:   headerChunk.id,
		statements: stmt.Consequence.Body.Statements,
	}

	var entryChunkID int
	remainingChunks, _, entryChunkID = splitBooleanExpressionChunks(stmt.Consequence.Expression, chunkCounter, consequenceChunk.id, returnID, remainingChunks, -1)
	headerChunk.branchBehavior = &jump{destChunkID: entryChunkID}
	remainingChunks = append(remainingChunks, consequenceChunk)
	remainingChunks = append(remainingChunks, headerChunk)

	return remainingChunks, &jump{destChunkID: headerChunk.id}, returnID
}

func createDoWhileStatementChunks(stmt *ast.DoWhileStatement, i int, curChunk *chunk, remainingChunks []*chunk, chunkCounter *int) ([]*chunk, *jump, int) {
	remainingChunks, returnID := curChunk.splitChunkForBranch(i, chunkCounter, remainingChunks)

	*chunkCounter++
	headerChunk := &chunk{
		id:         *chunkCounter,
		returnID:   returnID,
		statements: []ast.Statement{},
	}

	*chunkCounter++
	consequenceChunk := &chunk{
		id:         *chunkCounter,
		returnID:   headerChunk.id,
		statements: stmt.Consequence.Body.Statements,
	}

	var entryChunkID int
	remainingChunks, _, entryChunkID = splitBooleanExpressionChunks(stmt.Consequence.Expression, chunkCounter, consequenceChunk.id, returnID, remainingChunks, -1)
	headerChunk.branchBehavior = &jump{destChunkID: entryChunkID}
	remainingChunks = append(remainingChunks, consequenceChunk)
	remainingChunks = append(remainingChunks, headerChunk)

	return remainingChunks, &jump{destChunkID: consequenceChunk.id}, returnID
}

func (e *Emitter) renderChunks(chunks map[int]*chunk, scriptName string) string {
	// Get sorted list of final chunk ids.
	var chunkIDs []int
	if e.optimize {
		chunkIDs = optimizeChunkOrder(chunks)
	} else {
		chunkIDs = make([]int, 0)
		for k := range chunks {
			chunkIDs = append(chunkIDs, k)
		}
		sort.Ints(chunkIDs)
	}

	// First, render the bodies of each chunk. We'll
	// render the actual chunk labels after, since there is
	// an opportunity to skip renering unnecessary labels.
	var nextChunkID int
	chunkBodies := make(map[int]*strings.Builder)
	jumpChunks := make(map[int]bool)
	registerJumpChunk := func(chunkID int) {
		jumpChunks[chunkID] = true
	}
	for i, chunkID := range chunkIDs {
		var sb strings.Builder
		chunkBodies[chunkID] = &sb
		if i < len(chunkIDs)-1 {
			nextChunkID = chunkIDs[i+1]
		} else {
			nextChunkID = -1
		}
		chunk := chunks[chunkID]
		chunk.renderStatements(&sb)
		isFallThrough := chunk.renderBranching(scriptName, &sb, nextChunkID, registerJumpChunk)
		if !isFallThrough {
			sb.WriteString("\n")
		}
	}

	// Render the labels of each chunk, followed by its body.
	// A label doesn't need to be rendered if nothing ever jumps
	// to it.
	var sb strings.Builder
	for _, chunkID := range chunkIDs {
		chunk := chunks[chunkID]
		if chunkID == 0 || jumpChunks[chunkID] {
			chunk.renderLabel(scriptName, &sb)
		}
		sb.WriteString(chunkBodies[chunkID].String())
	}

	return sb.String()
}

// Reorders chunks to take advantage of fall-throughs, rather than using
// unncessary wasteful "goto" commands.
func optimizeChunkOrder(chunks map[int]*chunk) []int {
	unvisited := make(map[int]bool)
	for k := range chunks {
		unvisited[k] = true
	}

	chunkIDs := make([]int, 0)
	if len(chunks) == 0 {
		return chunkIDs
	}

	chunkIDs = append(chunkIDs, 0)
	delete(unvisited, 0)
	i := 1
	for len(chunkIDs) < len(chunks) {
		curChunk := chunks[chunkIDs[len(chunkIDs)-1]]
		var nextChunkID int
		if curChunk.branchBehavior != nil {
			nextChunkID = curChunk.branchBehavior.getTailChunkID()
		} else {
			nextChunkID = curChunk.returnID
		}

		if nextChunkID != -1 {
			if _, ok := unvisited[nextChunkID]; ok {
				chunkIDs = append(chunkIDs, nextChunkID)
				delete(unvisited, nextChunkID)
				continue
			}
		}

		// Choose random unvisited chunk for the next one.
		for i < len(chunks) {
			_, ok := unvisited[i]
			if ok {
				chunkIDs = append(chunkIDs, i)
				delete(unvisited, i)
				break
			}
			i++
		}
	}
	return chunkIDs
}

func emitText(text ast.Text) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s:\n", text.Name))
	lines := strings.Split(text.Value, "\n")
	for _, line := range lines {
		sb.WriteString(fmt.Sprintf("\t.string \"%s\"\n", line))
	}
	return sb.String()
}

func emitRawStatement(rawStmt *ast.RawStatement) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n", rawStmt.Value))
	return sb.String()
}
