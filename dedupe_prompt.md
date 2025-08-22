# Advanced Codebase Pattern Detection & Deduplication Analysis

You are an elite code archaeologist and pattern recognition specialist. Your previous analysis was surface-level. I need you to perform a **forensic-grade deep dive** into my codebase to uncover ALL forms of duplication - from obvious copy-paste to subtle behavioral patterns that create hidden maintenance debt.

## Mission: Leave No Pattern Unturned

Your goal is to identify every form of repetition, redundancy, and duplicated intent across the entire codebase. Think like a detective looking for clues that previous analyses missed.

## Advanced Pattern Recognition Framework

### 1. Multi-Dimensional Duplication Hunting

**Exact Code Duplicates (Basic Level):**
- Identical functions/methods with different names
- Copy-pasted blocks with minor variable substitutions
- Repeated constant values and magic numbers
- Duplicate type definitions and data structures

**Structural Pattern Duplicates (Intermediate Level):**
- Similar class/struct hierarchies solving the same problems
- Repeated design patterns (factory, builder, observer, etc.)
- Parallel inheritance chains or interface implementations
- Similar file/directory organization patterns

**Behavioral Pattern Duplicates (Advanced Level):**
- Functions with different implementations but identical logical outcomes
- Similar error handling strategies scattered across modules
- Repeated business rule implementations with slight variations
- Common algorithmic approaches reimplemented multiple times

**Intent-Based Duplicates (Expert Level):**
- Multiple solutions to the same conceptual problem
- Redundant abstractions that serve identical purposes
- Competing implementations of similar features
- Overlapping responsibilities between modules/classes

### 2. Forensic Analysis Techniques

**Code Fingerprinting:**
- Generate abstract syntax tree (AST) fingerprints for functions
- Compare control flow patterns and complexity metrics
- Identify recurring cyclomatic complexity signatures
- Map data flow patterns across modules

**Behavioral Analysis:**
- Trace function call chains to identify parallel execution paths
- Map input/output relationships for similar functions
- Identify shared dependency clusters
- Analyze exception handling and error propagation patterns

**Semantic Pattern Mining:**
- Group functions by conceptual purpose regardless of implementation
- Identify business logic scattered across architectural layers
- Find validation rules implemented in multiple locations
- Discover configuration and setup patterns

**Cross-Cutting Concern Detection:**
- Authentication/authorization implementations
- Logging and monitoring patterns
- Caching strategies and implementations
- Data validation and sanitization approaches
- Resource management and cleanup patterns

### 3. Hidden Duplication Indicators

**Naming Pattern Analysis:**
- Functions/variables with similar naming conventions
- Methods that end with similar suffixes (`...Handler`, `...Processor`, `...Manager`)
- Classes/modules with parallel naming schemes
- Configuration keys that suggest repeated functionality

**Import Pattern Analysis:**
- Files that import the same sets of dependencies
- Modules that require similar external libraries
- Repeated import aliasing patterns
- Circular or redundant dependency relationships

**Comment and Documentation Patterns:**
- Similar TODO comments indicating repeated technical debt
- Repeated explanatory comments for similar complex logic
- Documentation that describes the same concepts multiple times
- Code comments that apologize for duplication or complexity

**Test Pattern Analysis:**
- Similar test setups and teardown procedures
- Repeated mock implementations and test data
- Parallel test case structures for similar functionality
- Common assertion patterns and validation logic

### 4. Architectural Smell Detection

**Layer Violations:**
- Business logic duplicated across presentation and data layers
- Data access patterns reimplemented in multiple services
- Cross-cutting concerns handled differently in parallel modules

**Abstraction Failures:**
- Multiple concrete implementations where an interface would suffice
- Repeated composition patterns that could be abstracted
- Similar factory or builder patterns for different object types

**Configuration Redundancy:**
- Environment-specific settings duplicated across files
- Similar database connection or API client configurations
- Repeated feature flag or conditional logic patterns

## Enhanced Scanning Methodology

### 1. Multi-Pass Analysis

**Pass 1: Syntactic Similarity**
- Use fuzzy matching algorithms to find similar code blocks
- Identify functions with similar signatures but different names
- Find repeated code patterns with minor variations
- Detect copied code blocks with cosmetic changes

**Pass 2: Semantic Analysis**
- Group functions by their actual behavior regardless of implementation
- Identify conceptually equivalent operations
- Find business rules implemented multiple ways
- Discover redundant data transformations

**Pass 3: Architectural Pattern Mining**
- Map component interaction patterns
- Identify repeated architectural solutions
- Find parallel service implementations
- Discover redundant middleware or interceptor patterns

**Pass 4: Cross-Reference Validation**
- Verify that identified patterns are truly duplicative
- Eliminate false positives (intentional variations)
- Assess the coupling implications of consolidation
- Prioritize based on actual maintenance impact

### 2. Pattern Scoring Matrix

For each identified pattern, calculate:

**Duplication Severity Score (1-10):**
- Code similarity percentage
- Frequency of occurrence
- Lines of code impact
- Maintenance complexity

**Refactoring Difficulty Score (1-10):**
- Coupling to other systems
- Test coverage requirements
- Breaking change implications
- Domain complexity

**Business Impact Score (1-10):**
- Bug propagation risk
- Development velocity impact
- Code review complexity
- Onboarding difficulty for new developers

## Deep Dive Requirements

### 1. Pattern Taxonomy

**Infrastructure Patterns:**
- Database access and ORM patterns
- HTTP client configurations and retry logic
- Message queue producers and consumers
- File I/O and resource management

**Business Logic Patterns:**
- Validation rule implementations
- Data transformation pipelines
- Calculation and computation logic
- Workflow and state management

**Integration Patterns:**
- API client implementations
- External service adapters
- Protocol handlers and parsers
- Authentication and authorization flows

**UI/Presentation Patterns:**
- Form validation and submission logic
- Data formatting and display utilities
- Event handling and user interaction patterns
- Component composition and layout structures

### 2. Anti-Pattern Detection

**Shotgun Surgery Indicators:**
- Changes that require modifications across many files
- Business rules scattered across multiple modules
- Configuration changes that ripple through the system

**God Object Indicators:**
- Modules that handle too many responsibilities
- Classes/files that have grown to handle multiple concerns
- Utilities that have become catch-all dumping grounds

**Copy-Paste Programming Evidence:**
- Similar bugs fixed in multiple locations
- Parallel feature implementations with slight variations
- Repeated refactoring patterns across different modules

## Revolutionary Output Format: dedupe.md

```markdown
# Deep Codebase Archaeology: Complete Duplication Analysis

## Executive Summary
- **Duplication Density:** [patterns per 1000 LOC]
- **Hidden Debt:** [estimated hours of redundant maintenance per quarter]
- **Refactoring ROI:** [estimated productivity gain percentage]
- **Risk Heat Map:** [critical/high/medium/low pattern distribution]

## Pattern Discovery Report

### Tier 1: Critical Architectural Duplications
[Deep patterns that indicate fundamental design issues]

### Tier 2: Business Logic Redundancies  
[Repeated domain concepts implemented differently]

### Tier 3: Infrastructure Pattern Duplications
[Repeated technical implementations and utilities]

### Tier 4: Tactical Code Duplications
[Copy-paste scenarios and similar implementations]

## Pattern Consolidation Masterplan

### Phase 1: Foundation Stabilization (Critical Infrastructure)
[Most fundamental patterns that other code depends on]

### Phase 2: Business Logic Unification (Domain Consolidation)
[Core business concepts and domain logic]

### Phase 3: Infrastructure Harmonization (Technical Debt)
[Technical utilities and infrastructure patterns]

### Phase 4: Tactical Cleanup (Code Quality)
[Surface-level duplications and polish]

## Strategic Implementation Guide

### Consolidation Architectures
[Detailed technical approaches for each pattern type]

### Risk Mitigation Strategies
[Comprehensive safety nets for complex refactoring]

### Validation Frameworks
[How to prove the refactoring maintains correctness]

## Pattern Prevention System

### Architectural Guardrails
[Design principles to prevent future duplication]

### Development Process Integration
[How to catch patterns before they become problems]

### Automation and Tooling
[Continuous monitoring for emerging patterns]

## Detailed Evidence Portfolio

### Code Similarity Heat Maps
[Visual representation of duplication hotspots]

### Dependency Coupling Analysis
[How duplications create hidden dependencies]

### Maintenance Cost Projections
[Quantified impact of addressing each pattern]
```

## Critical Success Factors

**Depth Over Breadth:**
- Don't just find obvious duplications - discover the hidden patterns that slow development
- Look for conceptual duplications that might have completely different implementations
- Identify architectural patterns that create unnecessary complexity

**Context-Aware Analysis:**
- Understand WHY duplication exists before recommending consolidation
- Recognize when apparent duplication serves different domains or contexts
- Distinguish between harmful duplication and beneficial decoupling

**Implementation Realism:**
- Provide effort estimates that account for the true complexity of refactoring
- Consider the human factors: team knowledge, deployment constraints, business priorities
- Balance perfectionism with pragmatic incremental improvement

## Specific Investigation Directives

1. **Trace Similar Error Messages:** Find everywhere similar error conditions are handled
2. **Map Configuration Patterns:** Identify repeated setup and initialization logic
3. **Discover Hidden State Machines:** Find similar workflows implemented differently
4. **Uncover Validation Redundancy:** Locate business rules validated in multiple places
5. **Find Parallel Type Systems:** Identify competing ways to model the same concepts
6. **Detect Scattered Cross-Cutting Concerns:** Locate infrastructure concerns mixed into business logic
7. **Identify Competing Abstractions:** Find multiple ways to solve the same architectural problems

## Quality Gates

Your analysis must:
- Identify at least 3x more patterns than the initial surface analysis
- Provide concrete evidence for each claimed duplication
- Distinguish between helpful abstraction opportunities and dangerous coupling risks
- Include effort estimates accurate to within 25%
- Propose consolidation strategies that actually reduce complexity rather than just moving it around

## Final Challenge

Prove that you understand this codebase better than its original authors by finding the duplications they didn't even realize they created. Show me the hidden patterns that are making development slower than it needs to be.

Now, conduct your forensic analysis and create the definitive `dedupe.md` that will serve as the blueprint for transforming this codebase into a model of architectural clarity.
